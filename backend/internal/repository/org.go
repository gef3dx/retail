package repository

import (
	"context"

	"retail-backend/internal/model"
)

// OrgRepo — организации, роли (справочник), аудит-чтение.
type OrgRepo struct{}

func (OrgRepo) List(ctx context.Context, db DBTX) []model.Organization {
	rows, err := db.Query(ctx, `
		SELECT id, inn, kpp, full_name, short_name, tax_system, phone, email, is_active, created_at
		FROM organization WHERE deleted_at IS NULL ORDER BY id LIMIT 100`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Organization
	for rows.Next() {
		var o model.Organization
		_ = rows.Scan(&o.ID, &o.INN, &o.KPP, &o.FullName, &o.ShortName, &o.TaxSystem, &o.Phone, &o.Email, &o.IsActive, &o.CreatedAt)
		out = append(out, o)
	}
	if out == nil {
		out = []model.Organization{}
	}
	return out
}

func (OrgRepo) Create(ctx context.Context, db DBTX, inn, kpp, fullName, shortName string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO organization(inn,kpp,full_name,short_name) VALUES($1,$2,$3,NULLIF($4,'')) RETURNING id`,
		inn, kpp, fullName, shortName).Scan(&id)
	return id, err
}

func (OrgRepo) ListRoles(ctx context.Context, db DBTX) []model.Role {
	rows, err := db.Query(ctx, `SELECT id, name, display_name FROM roles ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Role
	for rows.Next() {
		var r model.Role
		_ = rows.Scan(&r.ID, &r.Name, &r.DisplayName)
		out = append(out, r)
	}
	return out
}

// EnsureDefaults создает зависимые дефолты новой организации:
// типы цен (каталог), настройки ГИС МТ и уведомлений. Идемпотентно.
func (OrgRepo) EnsureDefaults(ctx context.Context, db DBTX, orgID int64) {
	_, _ = db.Exec(ctx, `
		INSERT INTO price_type(organization_id, code, name, price_kind, is_default)
		VALUES ($1,'RETAIL','Розничная','RETAIL',TRUE),
		       ($1,'WHOLESALE','Оптовая','WHOLESALE',FALSE),
		       ($1,'PURCHASE','Закупочная','PURCHASE',FALSE)
		ON CONFLICT (organization_id, code) DO NOTHING`, orgID)
	_, _ = db.Exec(ctx, `INSERT INTO gismt_settings(organization_id) VALUES($1)
		ON CONFLICT (organization_id, provider) DO NOTHING`, orgID)
	_, _ = db.Exec(ctx, `INSERT INTO notify_settings(organization_id) VALUES($1) ON CONFLICT DO NOTHING`, orgID)
}
