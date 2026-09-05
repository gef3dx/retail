package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/model"
	"retail-backend/internal/service"
)

// Booking — тонкий слой над BookingService.
type Booking struct {
	Svc *service.BookingService
}

func (h *Booking) ListResources(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListResources(c.Request().Context(), orgID))
}

func (h *Booking) CreateResource(c echo.Context) error {
	var b service.CreateResourceInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateResource(c.Request().Context(), b)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Booking) GetSchedule(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.GetSchedule(c.Request().Context(), rid))
}

func (h *Booking) PutSchedule(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Days []struct {
			DOW    int    `json:"dow"`
			Start  string `json:"start"`
			End    string `json:"end"`
			Active bool   `json:"active"`
		} `json:"days"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	var days []model.SchedDay
	for _, d := range b.Days {
		days = append(days, model.SchedDay{DOW: d.DOW, Start: d.Start, End: d.End, Active: d.Active})
	}
	if err := h.Svc.PutSchedule(c.Request().Context(), rid, days); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Booking) AddException(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b service.ScheduleExceptionInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.AddException(c.Request().Context(), rid, b); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *Booking) Slots(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	durMin, _ := strconv.Atoi(c.QueryParam("duration"))
	slots, err := h.Svc.Slots(c.Request().Context(), rid, c.QueryParam("date"), durMin)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, slots)
}

func (h *Booking) Create(c echo.Context) error {
	var b service.CreateBookingInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	bid, err := h.Svc.Create(c.Request().Context(), b, ctxUser(c))
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": bid})
}

func (h *Booking) List(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.List(c.Request().Context(), orgID, c.QueryParam("date"), c.QueryParam("status")))
}

func (h *Booking) Get(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	d, err := h.Svc.Get(c.Request().Context(), id)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Booking) SetStatus(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.SetStatus(c.Request().Context(), id, b.Status, b.Comment, ctxUser(c)); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Booking) LinkReceipt(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		ReceiptItemID int64 `json:"receipt_item_id"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.LinkReceipt(c.Request().Context(), id, b.ReceiptItemID); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Booking) LinkProductResource(c echo.Context) error {
	pid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b service.LinkProductResourceInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.LinkProductResource(c.Request().Context(), pid, b); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Booking) ListProductResources(c echo.Context) error {
	pid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListProductResources(c.Request().Context(), pid))
}
