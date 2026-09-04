package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"retail-backend/internal/auth"
	"retail-backend/internal/middleware"
	"retail-backend/internal/model"
	"retail-backend/internal/store"
)

type Auth struct {
	Store      *store.Store
	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type registerReq struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	OrgName   string `json:"org_name"`
	OrgINN    string `json:"org_inn"`
	OrgKPP    string `json:"org_kpp"`
}

type loginReq struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func tokensFor(a *Auth, ctx echo.Context, uid int64, username string) (access, refresh string, accessExp, refreshExp time.Time, err error) {
	roles, err := a.Store.Roles(ctx.Request().Context(), uid)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	access, accessExp, err = auth.NewAccessToken(a.JWTSecret, uid, username, roles, a.AccessTTL)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	refresh, hash, err := auth.NewRefreshToken()
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	refreshExp = time.Now().Add(a.RefreshTTL)
	ip := clientIP(ctx)
	_, err = a.Store.PG.Exec(ctx.Request().Context(), `
		INSERT INTO user_sessions(user_id, refresh_hash, access_expires_at, refresh_expires_at, ip_address, user_agent)
		VALUES($1,$2,$3,$4,CAST(NULLIF($5,'') AS inet),$6)`,
		uid, hash, accessExp, refreshExp, ip, ctx.Request().UserAgent())
	return access, refresh, accessExp, refreshExp, err
}

// Register: создает организацию + пользователя ADMIN. Открытый эндпоинт (этап 2).
func (a *Auth) Register(c echo.Context) error {
	var r registerReq
	if err := c.Bind(&r); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	r.Username = strings.TrimSpace(r.Username)
	if len(r.Password) < 6 || r.Username == "" || r.Email == "" || r.OrgName == "" || r.OrgINN == "" || r.OrgKPP == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username/email/password>=6/org_name/org_inn/org_kpp required"})
	}
	hash, err := auth.HashPassword(r.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "hash failed"})
	}
	var uid int64
	err = a.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		var orgID int64
		if err := tx.QueryRow(c.Request().Context(), `
			INSERT INTO organization(inn,kpp,full_name,short_name) VALUES($1,$2,$3,$3) RETURNING id`,
			r.OrgINN, r.OrgKPP, r.OrgName).Scan(&orgID); err != nil {
			return err
		}
		if err := tx.QueryRow(c.Request().Context(), `
			INSERT INTO users(username,email,password_hash,first_name,last_name) VALUES($1,$2,$3,$4,$5) RETURNING id`,
			r.Username, r.Email, hash, r.FirstName, r.LastName).Scan(&uid); err != nil {
			return err
		}
		if _, err := tx.Exec(c.Request().Context(), `
			INSERT INTO user_organizations(user_id, organization_id, is_default) VALUES($1,$2,TRUE)`, uid, orgID); err != nil {
			return err
		}
		var roleID int64
		if err := tx.QueryRow(c.Request().Context(), `SELECT id FROM roles WHERE name='ADMIN'`).Scan(&roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(c.Request().Context(), `
			INSERT INTO user_roles(user_id, role_id, organization_id) VALUES($1,$2,$3)`, uid, roleID, orgID); err != nil {
			return err
		}
		if err := ensureDefaultPriceTypes(tx, c, orgID); err != nil {
			return err
		}
		EnsureGismtForOrgTx(tx, c, orgID)
		return nil
	})
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "register failed (duplicate?)"})
	}
	access, refresh, _, _, err := tokensFor(a, c, uid, r.Username)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "token failed"})
	}
	a.Store.Audit(c.Request().Context(), &uid, "register", "Регистрация с организацией", "user", &uid,
		map[string]string{"username": r.Username}, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]string{"access_token": access, "refresh_token": refresh})
}

// Login с защитой от перебора: 5 ошибок -> блокировка 15 минут.
func (a *Auth) Login(c echo.Context) error {
	var r loginReq
	if err := c.Bind(&r); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	ctx := c.Request().Context()
	var id int64
	var username, hash string
	var active, locked bool
	var fails int
	var lockedUntil *time.Time
	err := a.Store.PG.QueryRow(ctx, `
		SELECT id, username, password_hash, is_active, is_locked, failed_login_attempts, locked_until
		FROM users WHERE (username=$1 OR email=$1) AND deleted_at IS NULL`, r.Login).
		Scan(&id, &username, &hash, &active, &locked, &fails, &lockedUntil)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}
	if !active {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "user disabled"})
	}
	if locked || (lockedUntil != nil && lockedUntil.After(time.Now())) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "user locked, try later"})
	}
	if !auth.CheckPassword(hash, r.Password) {
		fails++
		_, _ = a.Store.PG.Exec(ctx, `UPDATE users SET failed_login_attempts=$1 WHERE id=$2`, fails, id)
		if fails >= 5 {
			_, _ = a.Store.PG.Exec(ctx, `UPDATE users SET is_locked=TRUE, locked_until=NOW()+INTERVAL '15 minutes', failed_login_attempts=0 WHERE id=$1`, id)
		}
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}
	ip := clientIP(c)
	_, _ = a.Store.PG.Exec(ctx, `UPDATE users SET failed_login_attempts=0, is_locked=FALSE, locked_until=NULL, last_login_at=NOW(), last_login_ip=CAST(NULLIF($1,'') AS inet) WHERE id=$2`, ip, id)
	access, refresh, _, _, err := tokensFor(a, c, id, username)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "token failed"})
	}
	a.Store.Audit(ctx, &id, "login", "Вход в систему", "user", &id, nil, ip, c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusOK, map[string]string{"access_token": access, "refresh_token": refresh})
}

func (a *Auth) Refresh(c echo.Context) error {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind(&body); err != nil || body.RefreshToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "refresh_token required"})
	}
	ctx := c.Request().Context()
	hash := auth.HashRefresh(body.RefreshToken)
	var sid, uid int64
	var username string
	var exp time.Time
	var active bool
	err := a.Store.PG.QueryRow(ctx, `
		SELECT s.id, s.user_id, u.username, s.refresh_expires_at, s.is_active
		FROM user_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.refresh_hash=$1`, hash).Scan(&sid, &uid, &username, &exp, &active)
	if err != nil || !active || exp.Before(time.Now()) {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid refresh"})
	}
	// Ротация: новый токен, старый деактивируем.
	newRefresh, newHash, err := auth.NewRefreshToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "token failed"})
	}
	roles, _ := a.Store.Roles(ctx, uid)
	access, accessExp, err := auth.NewAccessToken(a.JWTSecret, uid, username, roles, a.AccessTTL)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "token failed"})
	}
	refreshExp := time.Now().Add(a.RefreshTTL)
	_, err = a.Store.PG.Exec(ctx, `
		UPDATE user_sessions SET refresh_hash=$1, access_expires_at=$2, refresh_expires_at=$3 WHERE id=$4`,
		newHash, accessExp, refreshExp, sid)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "rotate failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"access_token": access, "refresh_token": newRefresh})
}

func (a *Auth) Logout(c echo.Context) error {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.Bind(&body)
	if body.RefreshToken != "" {
		_, _ = a.Store.PG.Exec(c.Request().Context(), `UPDATE user_sessions SET is_active=FALSE WHERE refresh_hash=$1`, auth.HashRefresh(body.RefreshToken))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (a *Auth) Me(c echo.Context) error {
	x := middleware.CtxOf(c)
	ctx := c.Request().Context()
	var u model.User
	var orgs []int64
	rows, _ := a.Store.PG.Query(ctx, `SELECT organization_id FROM user_organizations WHERE user_id=$1`, x.UserID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var o int64
			_ = rows.Scan(&o)
			orgs = append(orgs, o)
		}
	}
	err := a.Store.PG.QueryRow(ctx, `
		SELECT id, username, email, first_name, last_name, is_active, created_at FROM users WHERE id=$1`, x.UserID).
		Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName, &u.IsActive, &u.CreatedAt)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no user"})
	}
	u.Roles = x.Roles
	u.Organizations = orgs
	return c.JSON(http.StatusOK, u)
}

func clientIP(c echo.Context) string {
	ip := c.RealIP()
	if ip == "::1" {
		return "127.0.0.1"
	}
	return ip
}
