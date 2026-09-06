package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// Market — тонкий слой над MarketService.
type Market struct {
	Svc *service.MarketService
}

func (h *Market) Providers(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.Providers(c.Request().Context(), orgID))
}

func (h *Market) ListOffers(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListOffers(c.Request().Context(), orgID, c.QueryParam("provider")))
}

func (h *Market) CreateOffer(c echo.Context) error {
	var b service.CreateOfferInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateOffer(c.Request().Context(), b)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Market) DeleteOffer(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.Svc.DeleteOffer(c.Request().Context(), orgID, id); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Market) PullOrders(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var b struct {
		WarehouseID int64 `json:"warehouse_id"`
	}
	_ = c.Bind(&b)
	x := middleware.CtxOf(c)
	res, err := h.Svc.PullOrders(c.Request().Context(), orgID, c.Param("code"), b.WarehouseID, x.UserID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *Market) PushStocks(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var b struct {
		WarehouseID int64 `json:"warehouse_id"`
	}
	_ = c.Bind(&b)
	res, err := h.Svc.PushStocks(c.Request().Context(), orgID, c.Param("code"), b.WarehouseID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *Market) ListOrders(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListOrders(c.Request().Context(), orgID, c.QueryParam("provider")))
}

func (h *Market) ListSyncLog(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListSyncLog(c.Request().Context(), orgID, c.QueryParam("provider")))
}

// Egais — тонкий слой над EgaisService.
type Egais struct {
	Svc *service.EgaisService
}

func (h *Egais) Status(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	st, err := h.Svc.Status(c.Request().Context(), orgID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, st)
}

func (h *Egais) CreateDoc(c echo.Context) error {
	var b service.CreateEgaisDocInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	d, err := h.Svc.CreateDoc(c.Request().Context(), b, ctxUser(c))
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, d)
}

func (h *Egais) ListDocs(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.List(c.Request().Context(), orgID))
}
