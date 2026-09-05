package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/service"
)

// Stock — тонкий слой над StockService.
type Stock struct {
	Svc *service.StockService
}

func (h *Stock) ListCounterparties(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListCounterparties(c.Request().Context(), orgID, c.QueryParam("role")))
}

func (h *Stock) CreateCounterparty(c echo.Context) error {
	var b service.CreateCounterpartyInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateCounterparty(c.Request().Context(), b)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Stock) ListWarehouses(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListWarehouses(c.Request().Context(), orgID))
}

func (h *Stock) CreateWarehouse(c echo.Context) error {
	var b service.CreateWarehouseInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateWarehouse(c.Request().Context(), b)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Stock) Balances(c echo.Context) error {
	whID, _ := strconv.ParseInt(c.QueryParam("warehouse_id"), 10, 64)
	out, err := h.Svc.Balances(c.Request().Context(), whID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Stock) CreateReceiptDoc(c echo.Context) error {
	var b service.CreateReceiptDocInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateReceiptDoc(c.Request().Context(), b, ctxUser(c))
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Stock) PostReceiptDoc(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.Svc.PostReceiptDoc(c.Request().Context(), id); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "posted"})
}

func (h *Stock) ListReceiptDocs(c echo.Context) error {
	whID, _ := strconv.ParseInt(c.QueryParam("warehouse_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListReceiptDocs(c.Request().Context(), whID))
}

func (h *Stock) CreateOrder(c echo.Context) error {
	var b service.CreateOrderInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateOrder(c.Request().Context(), b, ctxUser(c))
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Stock) ListOrders(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.ListOrders(c.Request().Context(), c.QueryParam("status")))
}

func (h *Stock) GetOrder(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	d, err := h.Svc.GetOrder(c.Request().Context(), id)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Stock) ConfirmOrder(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.Svc.ConfirmOrder(c.Request().Context(), id); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "confirmed"})
}

func (h *Stock) CancelOrder(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.Svc.CancelOrder(c.Request().Context(), id); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "canceled"})
}

func (h *Stock) CreateShipment(c echo.Context) error {
	var b service.CreateShipmentInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	sid, err := h.Svc.CreateShipment(c.Request().Context(), b, ctxUser(c))
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": sid})
}

func (h *Stock) ListShipments(c echo.Context) error {
	orderID, _ := strconv.ParseInt(c.QueryParam("order_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListShipments(c.Request().Context(), orderID))
}
