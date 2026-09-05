package service

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/auth"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// AuthService — регистрация, логин, refresh, профиль.
type AuthService struct {
	Store      *store.Store
	Users      repository.UserRepo
	Orgs       repository.OrgRepo
	Audit      repository.AuditRepo
	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type Tokens struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

type RegisterInput struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	OrgName   string `json:"org_name"`
	OrgINN    string `json:"org_inn"`
	OrgKPP    string `json:"org_kpp"`
}

func (a *AuthService) issueTokens(ctx context.Context, db repository.DBTX, uid int64, username, ip, ua string) (Tokens, error) {
	var t Tokens
	roles := a.Users.Roles(ctx, db, uid)
	access, accessExp, err := auth.NewAccessToken(a.JWTSecret, uid, username, roles, a.AccessTTL)
	if err != nil {
		return t, err
	}
	refresh, hash, err := auth.NewRefreshToken()
	if err != nil {
		return t, err
	}
	refreshExp := time.Now().Add(a.RefreshTTL)
	if err := a.Users.CreateSession(ctx, db, uid, hash, accessExp, refreshExp, ip, ua); err != nil {
		return t, err
	}
	return Tokens{Access: access, Refresh: refresh}, nil
}

func (a *AuthService) Register(ctx context.Context, in RegisterInput, ip, ua string) (Tokens, error) {
	var out Tokens
	in.Username = strings.TrimSpace(in.Username)
	if len(in.Password) < 6 || in.Username == "" || in.Email == "" || in.OrgName == "" || in.OrgINN == "" || in.OrgKPP == "" {
		return out, BadRequest("username/email/password>=6/org_name/org_inn/org_kpp required")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return out, Internal("hash failed")
	}
	var uid, orgID int64
	err = a.Store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		orgID, err = a.Orgs.Create(ctx, tx, in.OrgINN, in.OrgKPP, in.OrgName, in.OrgName)
		if err != nil {
			return err
		}
		uid, err = a.Users.Create(ctx, tx, in.Username, in.Email, hash, in.FirstName, in.LastName)
		if err != nil {
			return err
		}
		a.Users.LinkOrg(ctx, tx, uid, orgID)
		roleID, err := a.Users.RoleID(ctx, tx, "ADMIN")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id, role_id, organization_id) VALUES($1,$2,$3)`,
			uid, roleID, orgID); err != nil {
			return err
		}
		a.Orgs.EnsureDefaults(ctx, tx, orgID)
		return nil
	})
	if err != nil {
		return out, Conflict("register failed (duplicate?)")
	}
	out, err = a.issueTokens(ctx, a.Store.PG, uid, in.Username, ip, ua)
	if err != nil {
		return out, Internal("token failed")
	}
	a.Audit.Log(ctx, a.Store.PG, &uid, "register", "Регистрация с организацией", "user", &uid,
		map[string]string{"username": in.Username}, ip, ua, true, "")
	_ = orgID
	return out, nil
}

func (a *AuthService) Login(ctx context.Context, login, password, ip, ua string) (Tokens, error) {
	var out Tokens
	u, err := a.Users.ByLogin(ctx, a.Store.PG, login)
	if err != nil {
		return out, Unauthorized("invalid credentials")
	}
	if !u.IsActive {
		return out, Forbidden("user disabled")
	}
	if u.IsLocked || (u.LockedUntil != nil && u.LockedUntil.After(time.Now())) {
		return out, Forbidden("user locked, try later")
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		a.Users.FailLogin(ctx, a.Store.PG, u.ID, u.FailedAttempts+1)
		return out, Unauthorized("invalid credentials")
	}
	a.Users.TouchLogin(ctx, a.Store.PG, u.ID, ip)
	out, err = a.issueTokens(ctx, a.Store.PG, u.ID, u.Username, ip, ua)
	if err != nil {
		return out, Internal("token failed")
	}
	a.Audit.Log(ctx, a.Store.PG, &u.ID, "login", "Вход в систему", "user", &u.ID, nil, ip, ua, true, "")
	return out, nil
}

func (a *AuthService) Refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	var out Tokens
	if refreshToken == "" {
		return out, BadRequest("refresh_token required")
	}
	s, err := a.Users.SessionByRefresh(ctx, a.Store.PG, auth.HashRefresh(refreshToken))
	if err != nil || !s.IsActive || s.RefreshExpiresAt.Before(time.Now()) {
		return out, Unauthorized("invalid refresh")
	}
	newRefresh, newHash, err := auth.NewRefreshToken()
	if err != nil {
		return out, Internal("token failed")
	}
	roles := a.Users.Roles(ctx, a.Store.PG, s.UserID)
	access, accessExp, err := auth.NewAccessToken(a.JWTSecret, s.UserID, s.Username, roles, a.AccessTTL)
	if err != nil {
		return out, Internal("token failed")
	}
	refreshExp := time.Now().Add(a.RefreshTTL)
	if err := a.Users.RotateSession(ctx, a.Store.PG, s.ID, newHash, accessExp, refreshExp); err != nil {
		return out, Internal("rotate failed")
	}
	return Tokens{Access: access, Refresh: newRefresh}, nil
}

func (a *AuthService) Logout(ctx context.Context, refreshToken string) {
	if refreshToken != "" {
		a.Users.DeactivateByRefresh(ctx, a.Store.PG, auth.HashRefresh(refreshToken))
	}
}

func (a *AuthService) Me(ctx context.Context, uid int64, roles []string) (model.User, error) {
	u, err := a.Users.Get(ctx, a.Store.PG, uid)
	if err != nil {
		return u, NotFound("no user")
	}
	u.Roles = roles
	u.Organizations = a.Users.Orgs(ctx, a.Store.PG, uid)
	return u, nil
}
