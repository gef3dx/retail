package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// Delivery — тонкий слой над DeliveryService.
type Delivery struct {
	Svc *service.DeliveryService
}

func (h *Delivery) ListZones(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListZones(c.Request().Context(), orgID))
}

func (h *Delivery) CreateZone(c echo.Context) error {
	var b service.CreateZoneInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateZone(c.Request().Context(), b)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Delivery) ListCouriers(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListCouriers(c.Request().Context(), orgID))
}

func (h *Delivery) CreateCourier(c echo.Context) error {
	var b service.CreateCourierInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateCourier(c.Request().Context(), b)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Delivery) SetCourierSchedule(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b service.CourierScheduleInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.SetCourierSchedule(c.Request().Context(), id, b); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Delivery) Create(c echo.Context) error {
	var b service.CreateDeliveryInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	id, err := h.Svc.Create(c.Request().Context(), b, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Delivery) List(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.List(c.Request().Context(), orgID, c.QueryParam("status")))
}

func (h *Delivery) Get(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	d, err := h.Svc.Get(c.Request().Context(), id)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Delivery) Assign(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		CourierID int64 `json:"courier_id"`
	}
	if err := c.Bind(&b); err != nil || b.CourierID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "courier_id required"})
	}
	x := middleware.CtxOf(c)
	if err := h.Svc.Assign(c.Request().Context(), id, b.CourierID, x.UserID, clientIP(c), c.Request().UserAgent()); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "assigned"})
}

func (h *Delivery) Accept(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Accept bool `json:"accept"`
	}
	_ = c.Bind(&b)
	x := middleware.CtxOf(c)
	if err := h.Svc.Accept(c.Request().Context(), id, b.Accept, x.UserID); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Delivery) SetStatus(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Status   string `json:"status"`
		Comment  string `json:"comment"`
		Tracking string `json:"tracking_number"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	if err := h.Svc.SetStatus(c.Request().Context(), id, b.Status, b.Comment, b.Tracking, x.UserID); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
