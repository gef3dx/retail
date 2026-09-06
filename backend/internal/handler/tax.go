package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/service"
)

// Tax — тонкий слой над TaxService.
type Tax struct {
	Svc *service.TaxService
}

func periodOf(c echo.Context) (int64, int, int) {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	year, _ := strconv.Atoi(c.QueryParam("year"))
	quarter, _ := strconv.Atoi(c.QueryParam("quarter"))
	return orgID, year, quarter
}

func (h *Tax) SalesBook(c echo.Context) error {
	orgID, year, quarter := periodOf(c)
	out, err := h.Svc.SalesBook(c.Request().Context(), int(orgID), year, quarter)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Tax) PurchaseBook(c echo.Context) error {
	orgID, year, quarter := periodOf(c)
	out, err := h.Svc.PurchaseBook(c.Request().Context(), int(orgID), year, quarter)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Tax) Close(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var b struct {
		Year     int    `json:"year"`
		Quarter  int    `json:"quarter"`
		DeclType string `json:"decl_type"`
	}
	_ = c.Bind(&b)
	d, err := h.Svc.ClosePeriod(c.Request().Context(), int(orgID), b.Year, b.Quarter, b.DeclType)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Tax) Declarations(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.Declarations(c.Request().Context(), orgID))
}

func (h *Tax) Submit(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.Svc.Submit(c.Request().Context(), orgID, id); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "submitted"})
}

func (h *Tax) Export(c echo.Context) error {
	orgID, year, quarter := periodOf(c)
	name, csv, err := h.Svc.ExportCSV(c.Request().Context(), int(orgID), year, quarter, c.Param("book"))
	if err != nil {
		return writeErr(c, err)
	}
	c.Response().Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+name)
	return c.String(http.StatusOK, csv)
}

func (h *Tax) Summary(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	return c.JSON(http.StatusOK, h.Svc.Summary(c.Request().Context(), orgID))
}
