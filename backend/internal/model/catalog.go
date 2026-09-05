package model

// Category — товарная категория.
type Category struct {
	ID              int64  `json:"id"`
	ParentID        *int64 `json:"parent_id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	MarkedByDefault bool   `json:"is_marked_by_default"`
	IsActive        bool   `json:"is_active"`
}

// Brand — бренд.
type Brand struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Country *string `json:"country,omitempty"`
}

// Product — товар или услуга каталога.
type Product struct {
	ID              int64    `json:"id"`
	SKU             string   `json:"sku"`
	GTIN            *string  `json:"gtin,omitempty"`
	Name            string   `json:"name"`
	CategoryID      *int64   `json:"category_id,omitempty"`
	BrandID         *int64   `json:"brand_id,omitempty"`
	MeasureUnit     string   `json:"measure_unit"`
	BasePrice       *float64 `json:"base_price,omitempty"`
	VATRate         *float64 `json:"vat_rate,omitempty"`
	IsMarked        bool     `json:"is_marked"`
	StatusCode      string   `json:"status_code"`
	RetailPrice     *float64 `json:"retail_price,omitempty"`
	ProductType     string   `json:"product_type"`
	ServiceDuration *int     `json:"service_duration_minutes,omitempty"`
	RequiresBooking bool     `json:"service_requires_booking"`
}

// Price — цена товара по типу цен.
type Price struct {
	ID            int64   `json:"id"`
	PriceType     string  `json:"price_type"`
	PriceTypeName string  `json:"price_type_name"`
	Price         float64 `json:"price"`
	ValidFrom     *string `json:"valid_from,omitempty"`
	ValidTo       *string `json:"valid_to,omitempty"`
}

// PriceType — тип цен организации.
type PriceType struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	PriceKind string `json:"price_kind"`
	IsDefault bool   `json:"is_default"`
}
