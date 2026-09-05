package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/model"
	"retail-backend/internal/service"
)

// Marking — тонкий слой над MarkingService.
type Marking struct {
	Svc *service.MarkingService
}

func (h *Marking) Register(c echo.Context) error {
	var b service.RegisterCodesInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	res, err := h.Svc.Register(c.Request().Context(), b, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, res)
}

func (h *Marking) List(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	out := h.Svc.List(c.Request().Context(), orgID,
		c.QueryParam("product_id"), c.QueryParam("status"), c.QueryParam("q"), limit)
	if out == nil {
		out = []model.MarkingCode{}
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Marking) Check(c echo.Context) error {
	res, err := h.Svc.Check(c.Request().Context(), c.Param("code"))
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *Marking) WriteOff(c echo.Context) error {
	var b struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	if err := h.Svc.WriteOff(c.Request().Context(), b.Code, b.Reason, x.Username); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "written_off"})
}

func (h *Marking) Queue(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.Queue(c.Request().Context(), orgID, c.QueryParam("status")))
}

func (h *Marking) Log(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.Log(c.Request().Context(), orgID, c.QueryParam("type")))
}
