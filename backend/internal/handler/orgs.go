package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// Orgs — тонкий слой над OrgService.
type Orgs struct {
	Svc *service.OrgService
}

func (h *Orgs) List(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.List(c.Request().Context()))
}

func (h *Orgs) Create(c echo.Context) error {
	var r struct {
		INN       string `json:"inn"`
		KPP       string `json:"kpp"`
		FullName  string `json:"full_name"`
		ShortName string `json:"short_name"`
	}
	if err := c.Bind(&r); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	id, err := h.Svc.Create(c.Request().Context(), r.INN, r.KPP, r.FullName, r.ShortName, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Orgs) Roles(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.Roles(c.Request().Context()))
}

func (h *Orgs) Audit(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.AuditLog(c.Request().Context()))
}
