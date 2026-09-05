package repository

import (
	"context"
	"time"

	"retail-backend/internal/model"
)

// UserRepo — пользователи, сессии, роли, связи. Все методы принимают DBTX.
type UserRepo struct{}

func (UserRepo) ByLogin(ctx context.Context, db DBTX, login string) (model.UserCredentials, error) {
	var u model.UserCredentials
	err := db.QueryRow(ctx, `
		SELECT id, username, password_hash, is_active, is_locked, failed_login_attempts, locked_until
		FROM users WHERE (username=$1 OR email=$1) AND deleted_at IS NULL`, login).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsActive, &u.IsLocked, &u.FailedAttempts, &u.LockedUntil)
	return u, err
}

func (UserRepo) Create(ctx context.Context, db DBTX, username, email, hash, first, last string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO users(username,email,password_hash,first_name,last_name) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		username, email, hash, first, last).Scan(&id)
	return id, err
}

func (UserRepo) TouchLogin(ctx context.Context, db DBTX, id int64, ip string) {
	_, _ = db.Exec(ctx, `UPDATE users SET failed_login_attempts=0, is_locked=FALSE, locked_until=NULL,
		last_login_at=NOW(), last_login_ip=CAST(NULLIF($1,'') AS inet) WHERE id=$2`, ip, id)
}

func (UserRepo) FailLogin(ctx context.Context, db DBTX, id int64, fails int) {
	_, _ = db.Exec(ctx, `UPDATE users SET failed_login_attempts=$1 WHERE id=$2`, fails, id)
	if fails >= 5 {
		_, _ = db.Exec(ctx, `UPDATE users SET is_locked=TRUE, locked_until=NOW()+INTERVAL '15 minutes',
			failed_login_attempts=0 WHERE id=$1`, id)
	}
}

func (UserRepo) Roles(ctx context.Context, db DBTX, userID int64) []string {
	rows, err := db.Query(ctx, `
		SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		_ = rows.Scan(&r)
		out = append(out, r)
	}
	return out
}

func (UserRepo) Permissions(ctx context.Context, db DBTX, userID int64) map[string]bool {
	out := map[string]bool{}
	rows, err := db.Query(ctx, `
		SELECT DISTINCT p.code
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		_ = rows.Scan(&code)
		out[code] = true
	}
	return out
}

func (UserRepo) Orgs(ctx context.Context, db DBTX, userID int64) []int64 {
	rows, err := db.Query(ctx, `SELECT organization_id FROM user_organizations WHERE user_id=$1`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var o int64
		_ = rows.Scan(&o)
		out = append(out, o)
	}
	return out
}

func (UserRepo) Get(ctx context.Context, db DBTX, id int64) (model.User, error) {
	var u model.User
	err := db.QueryRow(ctx, `
		SELECT id, username, email, first_name, last_name, is_active, created_at, telegram_chat_id, push_token FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName, &u.IsActive, &u.CreatedAt, &u.Telegram, &u.Push)
	return u, err
}

func (UserRepo) List(ctx context.Context, db DBTX) []model.User {
	rows, err := db.Query(ctx, `
		SELECT u.id, u.username, u.email, u.first_name, u.last_name, u.is_active, u.created_at,
		       COALESCE(STRING_AGG(DISTINCT r.name, ','), '') AS roles
		FROM users u LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE u.deleted_at IS NULL GROUP BY u.id ORDER BY u.id LIMIT 100`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.User
	for rows.Next() {
		var u model.User
		var roles string
		_ = rows.Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName, &u.IsActive, &u.CreatedAt, &roles)
		if roles != "" {
			u.Roles = splitCSV(roles)
		}
		out = append(out, u)
	}
	if out == nil {
		out = []model.User{}
	}
	return out
}

func (UserRepo) LinkOrg(ctx context.Context, db DBTX, userID, orgID int64) {
	_, _ = db.Exec(ctx, `
		INSERT INTO user_organizations(user_id, organization_id, is_default) VALUES($1,$2,TRUE) ON CONFLICT DO NOTHING`, userID, orgID)
}

func (UserRepo) GrantRole(ctx context.Context, db DBTX, userID int64, role string, orgID *int64) (bool, error) {
	res, err := db.Exec(ctx, `
		INSERT INTO user_roles(user_id, role_id, organization_id)
		SELECT $1, r.id, $2 FROM roles r WHERE r.name=$3 ON CONFLICT DO NOTHING`, userID, orgID, role)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

func (UserRepo) RoleID(ctx context.Context, db DBTX, name string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `SELECT id FROM roles WHERE name=$1`, name).Scan(&id)
	return id, err
}

// --- Sessions ---

func (UserRepo) CreateSession(ctx context.Context, db DBTX, userID int64, refreshHash string,
	accessExp, refreshExp time.Time, ip, ua string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO user_sessions(user_id, refresh_hash, access_expires_at, refresh_expires_at, ip_address, user_agent)
		VALUES($1,$2,$3,$4,CAST(NULLIF($5,'') AS inet),$6)`,
		userID, refreshHash, accessExp, refreshExp, ip, ua)
	return err
}

func (UserRepo) SessionByRefresh(ctx context.Context, db DBTX, hash string) (model.Session, error) {
	var s model.Session
	err := db.QueryRow(ctx, `
		SELECT s.id, s.user_id, u.username, s.refresh_expires_at, s.is_active
		FROM user_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.refresh_hash=$1`, hash).
		Scan(&s.ID, &s.UserID, &s.Username, &s.RefreshExpiresAt, &s.IsActive)
	return s, err
}

func (UserRepo) RotateSession(ctx context.Context, db DBTX, sessionID int64, newHash string, accessExp, refreshExp time.Time) error {
	_, err := db.Exec(ctx, `
		UPDATE user_sessions SET refresh_hash=$1, access_expires_at=$2, refresh_expires_at=$3 WHERE id=$4`,
		newHash, accessExp, refreshExp, sessionID)
	return err
}

func (UserRepo) DeactivateByRefresh(ctx context.Context, db DBTX, hash string) {
	_, _ = db.Exec(ctx, `UPDATE user_sessions SET is_active=FALSE WHERE refresh_hash=$1`, hash)
}

// UpdateProfile обновляет telegram/push адреса пользователя.
func (UserRepo) UpdateProfile(ctx context.Context, db DBTX, id int64, telegram, push *string) {
	_, _ = db.Exec(ctx, `
		UPDATE users SET telegram_chat_id=COALESCE($2, telegram_chat_id),
			push_token=COALESCE($3, push_token), updated_at=NOW() WHERE id=$1`, id, telegram, push)
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if part := s[start:i]; part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}
