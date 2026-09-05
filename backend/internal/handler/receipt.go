package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// Receipt — тонкий слой над ReceiptService.
type Receipt struct {
	Svc *service.ReceiptService
}

func (h *Receipt) ListRegisters(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListRegisters(c.Request().Context(), orgID))
}

func (h *Receipt) CreateRegister(c echo.Context) error {
	var b service.CreateRegisterInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateRegister(c.Request().Context(), b)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Receipt) PatchRegister(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.PatchRegister(c.Request().Context(), id, raw); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Receipt) OpenShift(c echo.Context) error {
	var b struct {
		CashRegisterID int64   `json:"cash_register_id"`
		StartCash      float64 `json:"start_cash"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	id, number, err := h.Svc.OpenShift(c.Request().Context(), b.CashRegisterID, b.StartCash, x.UserID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": id, "shift_number": number})
}

func (h *Receipt) OpenShiftForRegister(c echo.Context) error {
	regID, _ := strconv.ParseInt(c.QueryParam("register_id"), 10, 64)
	id, number, err := h.Svc.OpenShiftForRegister(c.Request().Context(), regID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": id, "shift_number": number})
}

func (h *Receipt) XReport(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rep, err := h.Svc.XReport(c.Request().Context(), id)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, rep)
}

func (h *Receipt) CloseShift(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		ActualCash *float64 `json:"actual_cash"`
	}
	_ = c.Bind(&b)
	x := middleware.CtxOf(c)
	z, err := h.Svc.CloseShift(c.Request().Context(), id, b.ActualCash, x.UserID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, z)
}

func (h *Receipt) Sell(c echo.Context) error {
	var b service.SellInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	rid, number, err := h.Svc.Sell(c.Request().Context(), b, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": rid, "receipt_number": number})
}

func (h *Receipt) Return(c echo.Context) error {
	var b service.ReturnInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	rid, number, err := h.Svc.Return(c.Request().Context(), b, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": rid, "receipt_number": number})
}

func (h *Receipt) Correction(c echo.Context) error {
	var b service.CorrectionInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	rid, number, err := h.Svc.Correction(c.Request().Context(), b, x.UserID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": rid, "receipt_number": number})
}

func (h *Receipt) ListReceipts(c echo.Context) error {
	shiftID, _ := strconv.ParseInt(c.QueryParam("shift_id"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	return c.JSON(http.StatusOK, h.Svc.ListReceipts(c.Request().Context(), shiftID, limit))
}

func (h *Receipt) GetOfdSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	st, err := h.Svc.GetOfdSettings(c.Request().Context(), orgID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, st)
}

func (h *Receipt) PatchOfdSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.PatchOfdSettings(c.Request().Context(), orgID, raw); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// TestFiscal — пробное пробитие через активного провайдера (ничего не сохраняет).
func (h *Receipt) TestFiscal(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Total float64 `json:"total"`
	}
	_ = c.Bind(&b)
	res, err := h.Svc.TestFiscal(c.Request().Context(), id, b.Total)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}

// ActiveProvider — активный ОФД-провайдер (без секретов, для бейджа кассы).
func (h *Receipt) ActiveProvider(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ActiveProvider(c.Request().Context(), orgID))
}
