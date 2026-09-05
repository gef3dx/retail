package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// Users — тонкий слой над UserService.
type Users struct {
	Svc *service.UserService
}

func (h *Users) List(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.List(c.Request().Context()))
}

func (h *Users) Create(c echo.Context) error {
	var r service.CreateUserInput
	if err := c.Bind(&r); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	id, err := h.Svc.Create(c.Request().Context(), r, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Users) AssignRole(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		Role           string `json:"role"`
		OrganizationID *int64 `json:"organization_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	if err := h.Svc.AssignRole(c.Request().Context(), id, body.Role, body.OrganizationID, x.UserID, clientIP(c), c.Request().UserAgent()); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
