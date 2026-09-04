package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/model"
	"retail-backend/internal/store"
)

type Orgs struct {
	Store *store.Store
}

func (h *Orgs) List(c echo.Context) error {
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, inn, kpp, full_name, short_name, tax_system, phone, email, is_active, created_at
		FROM organization WHERE deleted_at IS NULL ORDER BY id LIMIT 100`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
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
	return c.JSON(http.StatusOK, out)
}

type orgReq struct {
	INN       string `json:"inn"`
	KPP       string `json:"kpp"`
	FullName  string `json:"full_name"`
	ShortName string `json:"short_name"`
}

func (h *Orgs) Create(c echo.Context) error {
	var r orgReq
	if err := c.Bind(&r); err != nil || r.INN == "" || r.KPP == "" || r.FullName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "inn/kpp/full_name required"})
	}
	var id int64
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO organization(inn,kpp,full_name,short_name) VALUES($1,$2,$3,NULLIF($4,'')) RETURNING id`,
		r.INN, r.KPP, r.FullName, r.ShortName).Scan(&id)
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "duplicate inn"})
	}
	EnsurePriceTypesForOrg(h.Store, c, id)
	EnsureGismtForOrg(h.Store, c, id)
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "org.create", "Создание организации", "organization", &id, r, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Orgs) Roles(c echo.Context) error {
	rows, err := h.Store.PG.Query(c.Request().Context(), `SELECT id, name, display_name FROM roles ORDER BY id`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	var out []model.Role
	for rows.Next() {
		var r model.Role
		_ = rows.Scan(&r.ID, &r.Name, &r.DisplayName)
		out = append(out, r)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Orgs) Audit(c echo.Context) error {
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, user_id, action_type, action_description, entity_type, entity_id, is_success, created_at
		FROM audit_log ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var id int64
		var uid *int64
		var at, desc, et *string
		var eid *int64
		var ok bool
		var ts string
		_ = rows.Scan(&id, &uid, &at, &desc, &et, &eid, &ok, &ts)
		out = append(out, map[string]interface{}{"id": id, "user_id": uid, "action": at, "entity": et, "entity_id": eid, "ok": ok, "at": ts})
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	return c.JSON(http.StatusOK, out)
}
