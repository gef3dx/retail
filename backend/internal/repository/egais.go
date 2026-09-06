package repository

import (
	"context"

	"retail-backend/internal/model"
)

// EgaisRepo — документы ЕГАИС и настройки УТМ.
type EgaisRepo struct{}

func (EgaisRepo) Create(ctx context.Context, db DBTX, orgID int64, docType, xml string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO egais_document(organization_id, doc_type, body_xml) VALUES($1,$2,$3) RETURNING id`,
		orgID, docType, xml).Scan(&id)
	return id, err
}

func (EgaisRepo) SetResult(ctx context.Context, db DBTX, id int64, status, reply, errMsg string) {
	_, _ = db.Exec(ctx, `
		UPDATE egais_document SET status=$2, reply=NULLIF($3,''), error_message=NULLIF($4,''),
			sent_at=CASE WHEN $2::varchar IN ('SENT','ACCEPTED') THEN NOW() ELSE sent_at END
		WHERE id=$1`, id, status, reply, errMsg)
}

func (EgaisRepo) List(ctx context.Context, db DBTX, orgID int64) []model.EgaisDoc {
	rows, err := db.Query(ctx, `
		SELECT id, doc_type, status, reply, error_message, created_at::text
		FROM egais_document WHERE organization_id=$1 ORDER BY id DESC LIMIT 100`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.EgaisDoc
	for rows.Next() {
		var d model.EgaisDoc
		_ = rows.Scan(&d.ID, &d.Type, &d.Status, &d.Reply, &d.Error, &d.CreatedAt)
		out = append(out, d)
	}
	if out == nil {
		out = []model.EgaisDoc{}
	}
	return out
}
