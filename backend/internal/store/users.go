package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// Permissions возвращает множество кодов разрешений пользователя.
func (s *Store) Permissions(ctx context.Context, userID int64) (map[string]bool, error) {
	out := map[string]bool{}
	if s.PG == nil {
		return out, nil
	}
	rows, err := s.PG.Query(ctx, `
		SELECT DISTINCT p.code
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out[code] = true
	}
	return out, rows.Err()
}

// Roles возвращает имена ролей пользователя.
func (s *Store) Roles(ctx context.Context, userID int64) ([]string, error) {
	var roles []string
	if s.PG == nil {
		return roles, nil
	}
	rows, err := s.PG.Query(ctx, `
		SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// Audit пишет строку в audit_log. Ошибки игнорируются вызывающей стороной (best effort).
func (s *Store) Audit(ctx context.Context, userID *int64, action, desc, entity string, entityID *int64, newVals interface{}, ip, ua string, ok bool, errMsg string) {
	if s.PG == nil {
		return
	}
	var nb []byte
	if newVals != nil {
		nb, _ = json.Marshal(newVals)
	}
	_, _ = s.PG.Exec(ctx, `
		INSERT INTO audit_log(user_id, action_type, action_description, entity_type, entity_id, new_values, ip_address, user_agent, is_success, error_message)
		VALUES($1,$2,$3,$4,$5,$6,CAST(NULLIF($7,'') AS inet),$8,$9,NULLIF($10,''))`,
		userID, action, desc, entity, entityID, nb, ip, ua, ok, errMsg)
}

// Tx выполняет функцию в транзакции.
func (s *Store) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.PG.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
