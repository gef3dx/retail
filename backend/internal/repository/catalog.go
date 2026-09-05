package repository

import (
	"context"
	"strconv"

	"retail-backend/internal/model"
)

// CategoryRepo — справочник категорий.
type CategoryRepo struct{}

func (CategoryRepo) List(ctx context.Context, db DBTX) []model.Category {
	rows, err := db.Query(ctx, `
		SELECT id, parent_id, code, name, is_marked_by_default, is_active
		FROM catalog_category ORDER BY sort_order, name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Category
	for rows.Next() {
		var c model.Category
		_ = rows.Scan(&c.ID, &c.ParentID, &c.Code, &c.Name, &c.MarkedByDefault, &c.IsActive)
		out = append(out, c)
	}
	if out == nil {
		out = []model.Category{}
	}
	return out
}

func (CategoryRepo) Create(ctx context.Context, db DBTX, code, name string, parentID *int64, markedDefault bool) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO catalog_category(code, name, parent_id, is_marked_by_default) VALUES($1,$2,$3,$4) RETURNING id`,
		code, name, parentID, markedDefault).Scan(&id)
	return id, err
}

// BrandRepo — справочник брендов.
type BrandRepo struct{}

func (BrandRepo) List(ctx context.Context, db DBTX) []model.Brand {
	rows, err := db.Query(ctx, `SELECT id, name, country FROM catalog_brand WHERE is_active ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Brand
	for rows.Next() {
		var b model.Brand
		_ = rows.Scan(&b.ID, &b.Name, &b.Country)
		out = append(out, b)
	}
	if out == nil {
		out = []model.Brand{}
	}
	return out
}

func (BrandRepo) Create(ctx context.Context, db DBTX, name, country string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO catalog_brand(name, country) VALUES($1, NULLIF($2,'')) RETURNING id`, name, country).Scan(&id)
	return id, err
}

// ProductFilter — фильтры списка товаров.
type ProductFilter struct {
	Q          string
	CategoryID string
	BrandID    string
	MarkedOnly bool
	Status     string
	PType      string
	OrgID      int64
	Limit      int
	Offset     int
}

// retailPriceFrag подзапрос действующей розничной цены организации.
func retailPriceFrag(orgID int64) string {
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

// ProductRepo — товары и цены.
type ProductRepo struct{}

func (ProductRepo) List(ctx context.Context, db DBTX, f ProductFilter) []model.Product {
	args := []interface{}{f.Limit, f.Offset}
	where := "WHERE p.is_active = TRUE"
	if f.Q != "" {
		args = append(args, "%"+f.Q+"%")
		where += ` AND (p.name ILIKE $` + strconv.Itoa(len(args)) + ` OR p.sku ILIKE $` + strconv.Itoa(len(args)) + ` OR p.gtin ILIKE $` + strconv.Itoa(len(args)) + `)`
	}
	if f.CategoryID != "" {
		args = append(args, f.CategoryID)
		where += ` AND p.category_id = $` + strconv.Itoa(len(args))
	}
	if f.BrandID != "" {
		args = append(args, f.BrandID)
		where += ` AND p.brand_id = $` + strconv.Itoa(len(args))
	}
	if f.MarkedOnly {
		where += ` AND p.is_marked = TRUE`
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where += ` AND p.status_code = $` + strconv.Itoa(len(args))
	}
	if f.PType == "GOODS" || f.PType == "SERVICE" {
		args = append(args, f.PType)
		where += ` AND p.product_type = $` + strconv.Itoa(len(args))
	}
	rows, err := db.Query(ctx, `
		SELECT p.id, p.sku, p.gtin, p.name, p.category_id, p.brand_id, p.measure_unit,
		       p.base_price, p.vat_rate, p.is_marked, p.status_code,
		       p.product_type, p.service_duration_minutes, p.service_requires_booking, `+retailPriceFrag(f.OrgID)+`
		FROM catalog_product p `+where+` ORDER BY p.id LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Product
	for rows.Next() {
		var p model.Product
		_ = rows.Scan(&p.ID, &p.SKU, &p.GTIN, &p.Name, &p.CategoryID, &p.BrandID, &p.MeasureUnit,
			&p.BasePrice, &p.VATRate, &p.IsMarked, &p.StatusCode, &p.ProductType, &p.ServiceDuration, &p.RequiresBooking, &p.RetailPrice)
		out = append(out, p)
	}
	if out == nil {
		out = []model.Product{}
	}
	return out
}

func (ProductRepo) ByCode(ctx context.Context, db DBTX, code string, orgID int64) (model.Product, error) {
	var p model.Product
	err := db.QueryRow(ctx, `
		SELECT p.id, p.sku, p.gtin, p.name, p.measure_unit, p.base_price, p.vat_rate, p.is_marked, p.status_code, p.product_type, `+retailPriceFrag(orgID)+`
		FROM catalog_product p
		LEFT JOIN product_packaging pk ON pk.product_id = p.id AND pk.gtin_packaging = $1
		WHERE p.is_active = TRUE AND (p.sku = $1 OR p.gtin = $1 OR pk.gtin_packaging = $1)
		LIMIT 1`, code).Scan(&p.ID, &p.SKU, &p.GTIN, &p.Name, &p.MeasureUnit, &p.BasePrice, &p.VATRate, &p.IsMarked, &p.StatusCode, &p.ProductType, &p.RetailPrice)
	return p, err
}

// CreateInput — поля создания товара (совпадают с JSON API).
type CreateInput struct {
	SKU             string   `json:"sku"`
	GTIN            string   `json:"gtin"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	CategoryID      *int64   `json:"category_id"`
	BrandID         *int64   `json:"brand_id"`
	MeasureUnit     string   `json:"measure_unit"`
	BasePrice       *float64 `json:"base_price"`
	VATRate         *float64 `json:"vat_rate"`
	IsMarked        bool     `json:"is_marked"`
	MarkingType     string   `json:"marking_type"`
	StatusCode      string   `json:"status_code"`
	ProductType     string   `json:"product_type"`
	ServiceDuration *int     `json:"service_duration_minutes"`
	RequiresBooking *bool    `json:"service_requires_booking"`
	AllowWalkIn     *bool    `json:"service_allow_walk_in"`
	BookingEnabled  *bool    `json:"service_booking_enabled"`
	OrgID           *int64   `json:"org_id"`
	RetailPrice     *float64 `json:"retail_price"`
}

func (ProductRepo) Create(ctx context.Context, db DBTX, b CreateInput, status, ptype string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO catalog_product(sku, gtin, name, description, category_id, brand_id, measure_unit,
			base_price, vat_rate, is_marked, marking_type, status_code,
			product_type, service_duration_minutes, service_requires_booking,
			service_allow_walk_in, service_booking_enabled)
		VALUES($1, NULLIF($2,''), $3, NULLIF($4,''), $5, $6, COALESCE(NULLIF($7,''),'шт'),
			$8, COALESCE($9, 20.00), $10, NULLIF($11,''), $12,
			$13, $14, COALESCE($15, FALSE), COALESCE($16, TRUE), COALESCE($17, TRUE)) RETURNING id`,
		b.SKU, b.GTIN, b.Name, b.Description, b.CategoryID, b.BrandID, b.MeasureUnit,
		b.BasePrice, b.VATRate, b.IsMarked, b.MarkingType, status,
		ptype, b.ServiceDuration, b.RequiresBooking, b.AllowWalkIn, b.BookingEnabled).Scan(&id)
	if err != nil {
		return 0, err
	}
	if b.OrgID != nil && b.RetailPrice != nil {
		OrgRepo{}.EnsureDefaults(ctx, db, *b.OrgID)
		var ptID int64
		if err := db.QueryRow(ctx, `
			SELECT pt.id FROM price_type pt
			WHERE pt.organization_id = $1 AND pt.price_kind = 'RETAIL' AND pt.is_default`, *b.OrgID).Scan(&ptID); err != nil {
			return 0, err
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO product_price(product_id, price_type_id, price) VALUES($1,$2,$3)
			ON CONFLICT (product_id, price_type_id, valid_from) DO UPDATE SET price = EXCLUDED.price`,
			id, ptID, *b.RetailPrice); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// Update применяет частичное обновление из raw-карты (ключи = JSON-имена).
func (ProductRepo) Update(ctx context.Context, db DBTX, id int64, raw map[string]interface{}) (bool, error) {
	str := func(k string) interface{} {
		if v, ok := raw[k].(string); ok && v != "" {
			return v
		}
		return nil
	}
	num := func(k string) interface{} {
		if v, ok := raw[k].(float64); ok {
			return v
		}
		return nil
	}
	boolean := func(k string) interface{} {
		if v, ok := raw[k].(bool); ok {
			return v
		}
		return nil
	}
	intVal := func(k string) interface{} {
		if v, ok := raw[k].(float64); ok {
			return int(v)
		}
		return nil
	}
	var catID, brandID interface{}
	if v, ok := raw["category_id"].(float64); ok {
		catID = int64(v)
	}
	if v, ok := raw["brand_id"].(float64); ok {
		brandID = int64(v)
	}
	res, err := db.Exec(ctx, `
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
		id, str("name"), str("description"), catID, brandID, num("base_price"), num("vat_rate"),
		boolean("is_marked"), str("status_code"), str("product_type"), intVal("service_duration_minutes"),
		boolean("service_requires_booking"), boolean("service_allow_walk_in"), boolean("service_booking_enabled"))
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

func (ProductRepo) Deactivate(ctx context.Context, db DBTX, id int64) (bool, error) {
	res, err := db.Exec(ctx, `UPDATE catalog_product SET is_active=FALSE WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

// SaleProduct — товар для сборки позиции чека.
type SaleProduct struct {
	ID     int64
	Name   string
	SKU    string
	VAT    float64
	Marked bool
	Active bool
}

func (ProductRepo) ForSaleByID(ctx context.Context, db DBTX, id int64) (SaleProduct, error) {
	var p SaleProduct
	err := db.QueryRow(ctx, `
		SELECT id, name, sku, vat_rate, is_marked, is_active FROM catalog_product WHERE id=$1`, id).
		Scan(&p.ID, &p.Name, &p.SKU, &p.VAT, &p.Marked, &p.Active)
	return p, err
}

func (ProductRepo) ForSaleByCode(ctx context.Context, db DBTX, code string) (SaleProduct, error) {
	var p SaleProduct
	err := db.QueryRow(ctx, `
		SELECT p.id, p.name, p.sku, p.vat_rate, p.is_marked, p.is_active
		FROM catalog_product p LEFT JOIN product_packaging pk ON pk.product_id=p.id AND pk.gtin_packaging=$1
		WHERE p.sku=$1 OR p.gtin=$1 OR pk.gtin_packaging=$1 LIMIT 1`, code).
		Scan(&p.ID, &p.Name, &p.SKU, &p.VAT, &p.Marked, &p.Active)
	return p, err
}
func (ProductRepo) RetailPrice(ctx context.Context, db DBTX, productID, orgID int64) *float64 {
	var rp *float64
	_ = db.QueryRow(ctx, `
		SELECT pp.price FROM product_price pp JOIN price_type pt ON pt.id=pp.price_type_id
		WHERE pp.product_id=$1 AND pt.organization_id=$2 AND pt.price_kind='RETAIL'
		  AND pp.valid_from <= CURRENT_DATE AND (pp.valid_to IS NULL OR pp.valid_to >= CURRENT_DATE)
		ORDER BY pp.valid_from DESC LIMIT 1`, productID, orgID).Scan(&rp)
	return rp
}

func (ProductRepo) BasePrice(ctx context.Context, db DBTX, productID int64) *float64 {
	var bp *float64
	_ = db.QueryRow(ctx, `SELECT base_price FROM catalog_product WHERE id=$1`, productID).Scan(&bp)
	return bp
}

func (ProductRepo) Prices(ctx context.Context, db DBTX, productID int64) []model.Price {
	rows, err := db.Query(ctx, `
		SELECT pp.id, pt.code, pt.name, pp.price, pp.valid_from::text, pp.valid_to::text
		FROM product_price pp JOIN price_type pt ON pt.id = pp.price_type_id
		WHERE pp.product_id = $1 ORDER BY pp.valid_from DESC`, productID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Price
	for rows.Next() {
		var p model.Price
		_ = rows.Scan(&p.ID, &p.PriceType, &p.PriceTypeName, &p.Price, &p.ValidFrom, &p.ValidTo)
		out = append(out, p)
	}
	if out == nil {
		out = []model.Price{}
	}
	return out
}

func (ProductRepo) AddPrice(ctx context.Context, db DBTX, productID, priceTypeID int64, price float64, validFrom string) error {
	if validFrom == "" {
		_, err := db.Exec(ctx, `
			INSERT INTO product_price(product_id, price_type_id, price) VALUES($1,$2,$3)
			ON CONFLICT (product_id, price_type_id, valid_from) DO UPDATE SET price = EXCLUDED.price`,
			productID, priceTypeID, price)
		return err
	}
	_, err := db.Exec(ctx, `
		INSERT INTO product_price(product_id, price_type_id, price, valid_from) VALUES($1,$2,$3,$4::date)
		ON CONFLICT (product_id, price_type_id, valid_from) DO UPDATE SET price = EXCLUDED.price`,
		productID, priceTypeID, price, validFrom)
	return err
}

func (ProductRepo) PriceTypes(ctx context.Context, db DBTX, orgID int64) []model.PriceType {
	rows, err := db.Query(ctx, `
		SELECT id, code, name, price_kind, is_default FROM price_type WHERE organization_id=$1 AND is_active ORDER BY id`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.PriceType
	for rows.Next() {
		var t model.PriceType
		_ = rows.Scan(&t.ID, &t.Code, &t.Name, &t.PriceKind, &t.IsDefault)
		out = append(out, t)
	}
	if out == nil {
		out = []model.PriceType{}
	}
	return out
}
