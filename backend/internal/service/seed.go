package service

import (
	"context"
	"log/slog"

	"retail-backend/internal/auth"
	"retail-backend/internal/store"
)

// SeedAdmin создает SUPER_ADMIN из env, если пользователей еще нет.
func SeedAdmin(ctx context.Context, st *store.Store, email, password string) {
	if st.PG == nil {
		return
	}
	var count int
	if err := st.PG.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count > 0 {
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Error("seed hash failed", "err", err)
		return
	}
	var uid int64
	err = st.PG.QueryRow(ctx, `
		INSERT INTO users(username,email,password_hash,first_name,last_name) VALUES('superadmin',$1,$2,'Super','Admin') RETURNING id`,
		email, hash).Scan(&uid)
	if err != nil {
		slog.Error("seed admin failed", "err", err)
		return
	}
	_, _ = st.PG.Exec(ctx, `
		INSERT INTO user_roles(user_id, role_id) SELECT $1, r.id FROM roles r WHERE r.name='SUPER_ADMIN' ON CONFLICT DO NOTHING`, uid)
	slog.Info("seeded superadmin", "email", email)
}
