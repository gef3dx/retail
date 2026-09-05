package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/store"
)

type Catalog struct {
	Store *store.Store
}

// ensureDefaultPriceTypes создает RETAIL/WHOLESALE/PURCHASE для организации (идемпотентно).
func ensureDefaultPriceTypes(tx pgx.Tx, ctx echo.Context, orgID int64) error {
	_, err := tx.Exec(ctx.Request().Context(), `
		INSERT INTO price_type(organization_id, code, name, price_kind, is_default)
		VALUES
		 ($1, 'RETAIL', 'Розничная', 'RETAIL', TRUE),
		 ($1, 'WHOLESALE', 'Оптовая', 'WHOLESALE', FALSE),
		 ($1, 'PURCHASE', 'Закупочная', 'PURCHASE', FALSE)
		ON CONFLICT (organization_id, code) DO NOTHING`, orgID)
	return err
}

// EnsurePriceTypesForOrg — публичный вариант вне транзакции (для Orgs.Create).
func EnsurePriceTypesForOrg(s *store.Store, c echo.Context, orgID int64) {
	_, _ = s.PG.Exec(c.Request().Context(), `
		INSERT INTO price_type(organization_id, code, name, price_kind, is_default)
		VALUES
		 ($1, 'RETAIL', 'Розничная', 'RETAIL', TRUE),
		 ($1, 'WHOLESALE', 'Оптовая', 'WHOLESALE', FALSE),
		 ($1, 'PURCHASE', 'Закупочная', 'PURCHASE', FALSE)
		ON CONFLICT (organization_id, code) DO NOTHING`, orgID)
}

// ---------- Categories ----------

func (h *Catalog) ListCategories(c echo.Context) error {
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, parent_id, code, name, is_marked_by_default, is_active
		FROM catalog_category ORDER BY sort_order, name`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var parent *int64
		var code, name string
		var marked, active bool
		_ = rows.Scan(&id, &parent, &code, &name, &marked, &active)
		out = append(out, map[string]interface{}{"id": id, "parent_id": parent, "code": code, "name": name, "is_marked_by_default": marked, "is_active": active})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Catalog) CreateCategory(c echo.Context) error {
	var b struct {
		Code             string `json:"code"`
		Name             string `json:"name"`
		ParentID         *int64 `json:"parent_id"`
		IsMarkedByDefault bool  `json:"is_marked_by_default"`
	}
	if err := c.Bind(&b); err != nil || b.Code == "" || b.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "code/name required"})
	}
	var id int64
	if err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO catalog_category(code, name, parent_id, is_marked_by_default) VALUES($1,$2,$3,$4) RETURNING id`,
		b.Code, b.Name, b.ParentID, b.IsMarkedByDefault).Scan(&id); err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "duplicate code"})
	}
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "category.create", "Создание категории", "category", &id, b, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

// ---------- Brands ----------

func (h *Catalog) ListBrands(c echo.Context) error {
	rows, err := h.Store.PG.Query(c.Request().Context(), `SELECT id, name, country FROM catalog_brand WHERE is_active ORDER BY name`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name string
		var country *string
		_ = rows.Scan(&id, &name, &country)
		out = append(out, map[string]interface{}{"id": id, "name": name, "country": country})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Catalog) CreateBrand(c echo.Context) error {
	var b struct {
		Name    string `json:"name"`
		Country string `json:"country"`
	}
	if err := c.Bind(&b); err != nil || b.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name required"})
	}
	var id int64
	if err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO catalog_brand(name, country) VALUES($1, NULLIF($2,'')) RETURNING id`, b.Name, b.Country).Scan(&id); err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "duplicate name"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

// ---------- Products ----------

// priceFrag возвращает подзапрос розничной цены для организации.
func priceFrag(orgID int64) string {
	if orgID == 0 {
		return "NULL AS retail_price"
	}
	return `(SELECT pp.price FROM product_price pp
		JOIN price_type pt ON pt.id = pp.price_type_id
		WHERE pp.product_id = p.id AND pt.organization_id = ` + strconv.FormatInt(orgID, 10) + `
		  AND pt.price_kind = 'RETAIL' AND pp.valid_from <= CURRENT_DATE
		  AND (pp.valid_to IS NULL OR pp.valid_to >= CURRENT_DATE)
		ORDER BY pp.valid_from DESC LIMIT 1) AS retail_price`
}

func (h *Catalog) ListProducts(c echo.Context) error {
	q := strings.TrimSpace(c.QueryParam("q"))
	cat := c.QueryParam("category_id")
	brand := c.QueryParam("brand_id")
	marked := c.QueryParam("marked")
	status := c.QueryParam("status")
	ptype := c.QueryParam("type")
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	args := []interface{}{limit, offset}
	where := "WHERE p.is_active = TRUE"
	if q != "" {
		args = append(args, "%"+q+"%")
		where += ` AND (p.name ILIKE $` + strconv.Itoa(len(args)) + ` OR p.sku ILIKE $` + strconv.Itoa(len(args)) + ` OR p.gtin ILIKE $` + strconv.Itoa(len(args)) + `)`
	}
	if cat != "" {
		args = append(args, cat)
		where += ` AND p.category_id = $` + strconv.Itoa(len(args))
	}
	if brand != "" {
		args = append(args, brand)
		where += ` AND p.brand_id = $` + strconv.Itoa(len(args))
	}
	if marked == "true" {
		where += ` AND p.is_marked = TRUE`
	}
	if status != "" {
		args = append(args, status)
		where += ` AND p.status_code = $` + strconv.Itoa(len(args))
	}
	if ptype == "GOODS" || ptype == "SERVICE" {
		args = append(args, ptype)
		where += ` AND p.product_type = $` + strconv.Itoa(len(args))
	}

	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT p.id, p.sku, p.gtin, p.name, p.category_id, p.brand_id, p.measure_unit,
		       p.base_price, p.vat_rate, p.is_marked, p.status_code,
		       p.product_type, p.service_duration_minutes, p.service_requires_booking, `+priceFrag(orgID)+`
		FROM catalog_product p `+where+` ORDER BY p.id LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var sku, name, unit, st, ptyp string
		var gtin *string
		var catID, brandID *int64
		var base, vat *float64
		var marked, reqBook bool
		var dur *int
		var retail *float64
		_ = rows.Scan(&id, &sku, &gtin, &name, &catID, &brandID, &unit, &base, &vat, &marked, &st, &ptyp, &dur, &reqBook, &retail)
		out = append(out, map[string]interface{}{
			"id": id, "sku": sku, "gtin": gtin, "name": name, "category_id": catID,
			"brand_id": brandID, "measure_unit": unit, "base_price": base, "vat_rate": vat,
			"is_marked": marked, "status_code": st, "retail_price": retail,
			"product_type": ptyp, "service_duration_minutes": dur, "service_requires_booking": reqBook,
		})
	}
	return c.JSON(http.StatusOK, out)
}

// ByCode ищет товар по SKU / GTIN / штрихкоду упаковки. Главный метод кассира.
func (h *Catalog) ByCode(c echo.Context) error {
	code := strings.TrimSpace(c.Param("code"))
	if code == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "code required"})
	}
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var id int64
	var sku, name, unit, st string
	var gtin *string
	var base, vat *float64
	var marked bool
	var retail *float64
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT p.id, p.sku, p.gtin, p.name, p.measure_unit, p.base_price, p.vat_rate, p.is_marked, p.status_code, `+priceFrag(orgID)+`
		FROM catalog_product p
		LEFT JOIN product_packaging pk ON pk.product_id = p.id AND pk.gtin_packaging = $1
		WHERE p.is_active = TRUE AND (p.sku = $1 OR p.gtin = $1 OR pk.gtin_packaging = $1)
		LIMIT 1`, code).Scan(&id, &sku, &gtin, &name, &unit, &base, &vat, &marked, &st, &retail)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"id": id, "sku": sku, "gtin": gtin, "name": name, "measure_unit": unit,
		"base_price": base, "vat_rate": vat, "is_marked": marked, "status_code": st, "retail_price": retail,
	})
}

type productReq struct {
	SKU         string   `json:"sku"`
	GTIN        string   `json:"gtin"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CategoryID  *int64   `json:"category_id"`
	BrandID     *int64   `json:"brand_id"`
	MeasureUnit string   `json:"measure_unit"`
	BasePrice   *float64 `json:"base_price"`
	VATRate     *float64 `json:"vat_rate"`
	IsMarked    bool     `json:"is_marked"`
	MarkingType string   `json:"marking_type"`
	StatusCode  string   `json:"status_code"`
	// Услуга (этап 8)
	ProductType      string `json:"product_type"`
	ServiceDuration  *int   `json:"service_duration_minutes"`
	RequiresBooking  *bool  `json:"service_requires_booking"`
	AllowWalkIn      *bool  `json:"service_allow_walk_in"`
	BookingEnabled   *bool  `json:"service_booking_enabled"`
	// Опционально: сразу розничная цена для организации
	OrgID       *int64   `json:"org_id"`
	RetailPrice *float64 `json:"retail_price"`
}

func (h *Catalog) CreateProduct(c echo.Context) error {
	var b productReq
	if err := c.Bind(&b); err != nil || b.SKU == "" || b.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "sku/name required"})
	}
	if b.VATRate != nil && (*b.VATRate < 0 || *b.VATRate > 30) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "vat_rate 0..30"})
	}
	st := b.StatusCode
	if st == "" {
		st = "ACTIVE"
	}
	pt := b.ProductType
	if pt == "" {
		pt = "GOODS"
	}
	if pt != "GOODS" && pt != "SERVICE" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "product_type GOODS/SERVICE"})
	}
	var id int64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		if err := tx.QueryRow(ctx, `
			INSERT INTO catalog_product(sku, gtin, name, description, category_id, brand_id, measure_unit,
				base_price, vat_rate, is_marked, marking_type, status_code,
				product_type, service_duration_minutes, service_requires_booking,
				service_allow_walk_in, service_booking_enabled)
			VALUES($1, NULLIF($2,''), $3, NULLIF($4,''), $5, $6, COALESCE(NULLIF($7,''),'шт'),
				$8, COALESCE($9, 20.00), $10, NULLIF($11,''), $12,
				$13, $14, COALESCE($15, FALSE), COALESCE($16, TRUE), COALESCE($17, TRUE)) RETURNING id`,
			b.SKU, b.GTIN, b.Name, b.Description, b.CategoryID, b.BrandID, b.MeasureUnit,
			b.BasePrice, b.VATRate, b.IsMarked, b.MarkingType, st,
			pt, b.ServiceDuration, b.RequiresBooking, b.AllowWalkIn, b.BookingEnabled).Scan(&id); err != nil {
			return err
		}
		if b.OrgID != nil && b.RetailPrice != nil {
			if err := ensureDefaultPriceTypes(tx, c, *b.OrgID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO product_price(product_id, price_type_id, price)
				SELECT $1, pt.id, $3 FROM price_type pt
				WHERE pt.organization_id = $2 AND pt.price_kind = 'RETAIL' AND pt.is_default
				ON CONFLICT (product_id, price_type_id, valid_from) DO UPDATE SET price = EXCLUDED.price`,
				id, *b.OrgID, *b.RetailPrice); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "create failed (duplicate sku/gtin?)"})
	}
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "product.create", "Создание товара", "product", &id, b, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Catalog) UpdateProduct(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	str := func(k string) interface{} {
		if v, ok := raw[k].(string); ok && v != "" {
			return v
		}
		return nil
	}
	num := func(k string) interface{} {
		if v, ok := raw[k].(float64); ok {
			if k == "vat_rate" && (v < 0 || v > 30) {
				return "bad"
			}
			return v
		}
		return nil
	}
	if num("vat_rate") == "bad" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "vat_rate 0..30"})
	}
	var marked interface{}
	if v, ok := raw["is_marked"].(bool); ok {
		marked = v
	}
	boolean := func(k string) interface{} {
		if v, ok := raw[k].(bool); ok {
			return v
		}
		return nil
	}
	var catID, brandID, duration interface{}
	if v, ok := raw["category_id"].(float64); ok {
		catID = int64(v)
	}
	if v, ok := raw["brand_id"].(float64); ok {
		brandID = int64(v)
	}
	if v, ok := raw["service_duration_minutes"].(float64); ok {
		duration = int(v)
	}
	ptype := str("product_type")
	if ptype != nil && ptype != "GOODS" && ptype != "SERVICE" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "product_type GOODS/SERVICE"})
	}
	res, err := h.Store.PG.Exec(c.Request().Context(), `
		UPDATE catalog_product SET
			name = COALESCE($2::text, name),
			description = COALESCE($3::text, description),
			category_id = COALESCE($4::bigint, category_id),
			brand_id = COALESCE($5::bigint, brand_id),
			base_price = COALESCE($6::numeric, base_price),
			vat_rate = COALESCE($7::numeric, vat_rate),
			is_marked = COALESCE($8::boolean, is_marked),
			status_code = COALESCE($9::varchar, status_code),
			product_type = COALESCE($10::varchar, product_type),
			service_duration_minutes = COALESCE($11::int, service_duration_minutes),
			service_requires_booking = COALESCE($12::boolean, service_requires_booking),
			service_allow_walk_in = COALESCE($13::boolean, service_allow_walk_in),
			service_booking_enabled = COALESCE($14::boolean, service_booking_enabled),
			updated_at = NOW()
		WHERE id = $1 AND is_active = TRUE`,
		id, str("name"), str("description"), catID, brandID, num("base_price"), num("vat_rate"), marked, str("status_code"),
		ptype, duration, boolean("service_requires_booking"), boolean("service_allow_walk_in"), boolean("service_booking_enabled"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "update failed"})
	}
	if res.RowsAffected() == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Catalog) DeleteProduct(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	res, err := h.Store.PG.Exec(c.Request().Context(), `UPDATE catalog_product SET is_active=FALSE WHERE id=$1`, id)
	if err != nil || res.RowsAffected() == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "product.delete", "Мягкое удаление товара", "product", &id, nil, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ---------- Prices ----------

func (h *Catalog) ListPrices(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT pp.id, pt.code, pt.name, pp.price, pp.valid_from::text, pp.valid_to::text
		FROM product_price pp JOIN price_type pt ON pt.id = pp.price_type_id
		WHERE pp.product_id = $1 ORDER BY pp.valid_from DESC`, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var pid int64
		var code, name string
		var price float64
		var from, to *string
		_ = rows.Scan(&pid, &code, &name, &price, &from, &to)
		out = append(out, map[string]interface{}{"id": pid, "price_type": code, "price_type_name": name, "price": price, "valid_from": from, "valid_to": to})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Catalog) AddPrice(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		PriceTypeID int64    `json:"price_type_id"`
		Price       float64  `json:"price"`
		ValidFrom   string   `json:"valid_from"`
	}
	if err := c.Bind(&b); err != nil || b.PriceTypeID == 0 || b.Price < 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "price_type_id/price>=0 required"})
	}
	vf := b.ValidFrom
	if vf == "" {
		vf = "CURRENT_DATE"
		_, err := h.Store.PG.Exec(c.Request().Context(), `
			INSERT INTO product_price(product_id, price_type_id, price) VALUES($1,$2,$3)
			ON CONFLICT (product_id, price_type_id, valid_from) DO UPDATE SET price = EXCLUDED.price`, id, b.PriceTypeID, b.Price)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "add price failed"})
		}
		return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
	}
	_, err := h.Store.PG.Exec(c.Request().Context(), `
		INSERT INTO product_price(product_id, price_type_id, price, valid_from) VALUES($1,$2,$3,$4::date)
		ON CONFLICT (product_id, price_type_id, valid_from) DO UPDATE SET price = EXCLUDED.price`, id, b.PriceTypeID, b.Price, vf)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "add price failed"})
	}
	return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *Catalog) ListPriceTypes(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	if orgID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id required"})
	}
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, code, name, price_kind, is_default FROM price_type WHERE organization_id=$1 AND is_active ORDER BY id`, orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var i int64
		var code, name, kind string
		var def bool
		_ = rows.Scan(&i, &code, &name, &kind, &def)
		out = append(out, map[string]interface{}{"id": i, "code": code, "name": name, "price_kind": kind, "is_default": def})
	}
	return c.JSON(http.StatusOK, out)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
