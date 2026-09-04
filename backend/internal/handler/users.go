package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/auth"
	"retail-backend/internal/middleware"
	"retail-backend/internal/model"
	"retail-backend/internal/store"
)

type Users struct {
	Store *store.Store
}

func (h *Users) List(c echo.Context) error {
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT u.id, u.username, u.email, u.first_name, u.last_name, u.is_active, u.created_at,
		       COALESCE(STRING_AGG(DISTINCT r.name, ','), '') AS roles
		FROM users u LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE u.deleted_at IS NULL GROUP BY u.id ORDER BY u.id LIMIT 100`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
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
	return c.JSON(http.StatusOK, out)
}

type createUserReq struct {
	Username       string `json:"username"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	OrganizationID *int64 `json:"organization_id"`
	Role           string `json:"role"`
}

func (h *Users) Create(c echo.Context) error {
	var r createUserReq
	if err := c.Bind(&r); err != nil || r.Username == "" || r.Email == "" || len(r.Password) < 6 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username/email/password>=6 required"})
	}
	hash, _ := auth.HashPassword(r.Password)
	var id int64
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO users(username,email,password_hash,first_name,last_name) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		r.Username, r.Email, hash, r.FirstName, r.LastName).Scan(&id)
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "duplicate username/email"})
	}
	if r.OrganizationID != nil {
		_, _ = h.Store.PG.Exec(c.Request().Context(), `
			INSERT INTO user_organizations(user_id, organization_id, is_default) VALUES($1,$2,TRUE) ON CONFLICT DO NOTHING`, id, *r.OrganizationID)
		role := r.Role
		if role == "" {
			role = "VIEWER"
		}
		_, _ = h.Store.PG.Exec(c.Request().Context(), `
			INSERT INTO user_roles(user_id, role_id, organization_id)
			SELECT $1, r.id, $2 FROM roles r WHERE r.name=$3 ON CONFLICT DO NOTHING`, id, *r.OrganizationID, role)
	}
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "user.create", "Создание пользователя", "user", &id, r, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Users) AssignRole(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		Role           string `json:"role"`
		OrganizationID *int64 `json:"organization_id"`
	}
	if err := c.Bind(&body); err != nil || body.Role == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "role required"})
	}
	res, err := h.Store.PG.Exec(c.Request().Context(), `
		INSERT INTO user_roles(user_id, role_id, organization_id)
		SELECT $1, r.id, $2 FROM roles r WHERE r.name=$3 ON CONFLICT DO NOTHING`, id, body.OrganizationID, body.Role)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "assign failed"})
	}
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "user.role", "Назначение роли "+body.Role, "user", &id, body, clientIP(c), c.Request().UserAgent(), res.RowsAffected() > 0, "")
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
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
