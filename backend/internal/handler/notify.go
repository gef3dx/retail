package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// Notify — тонкий слой над NotifyService.
type Notify struct {
	Svc *service.NotifyService
}

func (h *Notify) Inbox(c echo.Context) error {
	x := middleware.CtxOf(c)
	return c.JSON(http.StatusOK, h.Svc.Inbox(c.Request().Context(), x.UserID))
}

func (h *Notify) MarkViewed(c echo.Context) error {
	x := middleware.CtxOf(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.Svc.MarkViewed(c.Request().Context(), id, x.UserID); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "viewed"})
}

func (h *Notify) Queue(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.QueueList(c.Request().Context(), c.QueryParam("status")))
}

func (h *Notify) Templates(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.Templates(c.Request().Context()))
}

func (h *Notify) UpsertTemplate(c echo.Context) error {
	var b service.UpsertTemplateInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.UpsertTemplate(c.Request().Context(), b); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Notify) Preferences(c echo.Context) error {
	x := middleware.CtxOf(c)
	return c.JSON(http.StatusOK, h.Svc.Preferences(c.Request().Context(), x.UserID))
}

func (h *Notify) SetPreference(c echo.Context) error {
	x := middleware.CtxOf(c)
	var b struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.SetPreference(c.Request().Context(), x.UserID, b.Type, b.Channel, b.Enabled); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Notify) Send(c echo.Context) error {
	var b service.SendInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.Send(c.Request().Context(), b); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]string{"status": "queued"})
}

func (h *Notify) GetSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	st, err := h.Svc.GetSettings(c.Request().Context(), orgID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, st)
}

func (h *Notify) PatchSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	h.Svc.PatchSettings(c.Request().Context(), orgID, raw)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
