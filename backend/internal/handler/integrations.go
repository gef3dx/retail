package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/service"
)

// Integrations — тонкий слой над IntegrationService.
type Integrations struct {
	Svc *service.IntegrationService
}

func (h *Integrations) List(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	out, err := h.Svc.Statuses(c.Request().Context(), orgID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Integrations) Save(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var b service.SaveIntegrationInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.Save(c.Request().Context(), orgID, c.Param("code"), b); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Integrations) Clear(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	if err := h.Svc.Clear(c.Request().Context(), orgID, c.Param("code")); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Integrations) Test(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	ok, msg, err := h.Svc.Test(c.Request().Context(), orgID, c.Param("code"))
	if err != nil {
		return writeErr(c, err)
	}
	code := 200
	if !ok {
		code = 422
	}
	return c.JSON(code, map[string]interface{}{"ok": ok, "message": msg})
}
