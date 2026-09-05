package repository

import (
	"context"
	"encoding/json"
)

// AuditRepo — best-effort запись в audit_log (ошибки глушатся вызывающим).
type AuditRepo struct{}

func (AuditRepo) Log(ctx context.Context, db DBTX, userID *int64, action, desc, entity string,
	entityID *int64, newVals interface{}, ip, ua string, ok bool, errMsg string) {
	var nb []byte
	if newVals != nil {
		nb, _ = json.Marshal(newVals)
	}
	_, _ = db.Exec(ctx, `
		INSERT INTO audit_log(user_id, action_type, action_description, entity_type, entity_id, new_values, ip_address, user_agent, is_success, error_message)
		VALUES($1,$2,$3,$4,$5,$6,CAST(NULLIF($7,'') AS inet),$8,$9,NULLIF($10,''))`,
		userID, action, desc, entity, entityID, nb, ip, ua, ok, errMsg)
}

// Entry — строка журнала для чтения.
type Entry struct {
	ID       int64   `json:"id"`
	UserID   *int64  `json:"user_id"`
	Action   *string `json:"action"`
	Entity   *string `json:"entity"`
	EntityID *int64  `json:"entity_id"`
	OK       bool    `json:"ok"`
	At       string  `json:"at"`
}

func (AuditRepo) List(ctx context.Context, db DBTX) []Entry {
	rows, err := db.Query(ctx, `
		SELECT id, user_id, action_type, action_description, entity_type, entity_id, is_success, created_at
		FROM audit_log ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var desc *string
		_ = rows.Scan(&e.ID, &e.UserID, &e.Action, &desc, &e.Entity, &e.EntityID, &e.OK, &e.At)
		out = append(out, e)
	}
	if out == nil {
		out = []Entry{}
	}
	return out
}
