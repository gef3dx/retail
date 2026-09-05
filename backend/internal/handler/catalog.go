package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/repository"
	"retail-backend/internal/service"
)

// Catalog — тонкий слой над CatalogService.
type Catalog struct {
	Svc *service.CatalogService
}

func (h *Catalog) ListCategories(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.ListCategories(c.Request().Context()))
}

func (h *Catalog) CreateCategory(c echo.Context) error {
	var b struct {
		Code              string `json:"code"`
		Name              string `json:"name"`
		ParentID          *int64 `json:"parent_id"`
		IsMarkedByDefault bool   `json:"is_marked_by_default"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	id, err := h.Svc.CreateCategory(c.Request().Context(), b.Code, b.Name, b.ParentID, b.IsMarkedByDefault, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Catalog) ListBrands(c echo.Context) error {
	return c.JSON(http.StatusOK, h.Svc.ListBrands(c.Request().Context()))
}

func (h *Catalog) CreateBrand(c echo.Context) error {
	var b struct {
		Name    string `json:"name"`
		Country string `json:"country"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	id, err := h.Svc.CreateBrand(c.Request().Context(), b.Name, b.Country)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Catalog) ListProducts(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	f := repository.ProductFilter{
		Q:          strings.TrimSpace(c.QueryParam("q")),
		CategoryID: c.QueryParam("category_id"),
		BrandID:    c.QueryParam("brand_id"),
		MarkedOnly: c.QueryParam("marked") == "true",
		Status:     c.QueryParam("status"),
		PType:      c.QueryParam("type"),
		OrgID:      orgID,
		Limit:      limit,
		Offset:     offset,
	}
	return c.JSON(http.StatusOK, h.Svc.ListProducts(c.Request().Context(), f))
}

func (h *Catalog) ByCode(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	p, err := h.Svc.ByCode(c.Request().Context(), c.Param("code"), orgID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, p)
}

func (h *Catalog) CreateProduct(c echo.Context) error {
	var b repository.CreateInput
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	x := middleware.CtxOf(c)
	id, err := h.Svc.CreateProduct(c.Request().Context(), b, x.UserID, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Catalog) UpdateProduct(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.UpdateProduct(c.Request().Context(), id, raw); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Catalog) DeleteProduct(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	x := middleware.CtxOf(c)
	if err := h.Svc.DeleteProduct(c.Request().Context(), id, x.UserID, clientIP(c), c.Request().UserAgent()); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Catalog) ListPrices(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.ListPrices(c.Request().Context(), id))
}

func (h *Catalog) AddPrice(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		PriceTypeID int64   `json:"price_type_id"`
		Price       float64 `json:"price"`
		ValidFrom   string  `json:"valid_from"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := h.Svc.AddPrice(c.Request().Context(), id, b.PriceTypeID, b.Price, b.ValidFrom); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *Catalog) ListPriceTypes(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	out, err := h.Svc.ListPriceTypes(c.Request().Context(), orgID)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}
