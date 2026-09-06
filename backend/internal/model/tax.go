package model

// BookEntry — строка книги покупок/продаж.
type BookEntry struct {
	Number  int      `json:"entry_number"`
	DocType string   `json:"document_type"`
	DocNum  *string  `json:"document_number,omitempty"`
	DocDate *string  `json:"document_date,omitempty"`
	Counter *string  `json:"counterparty_inn,omitempty"`
	Amount  float64  `json:"amount"`
	VAT     float64  `json:"vat_amount"`
	Total   float64  `json:"total_amount"`
	VATRate *float64 `json:"vat_rate,omitempty"`
}

// Declaration — декларация за период.
type Declaration struct {
	ID         int64   `json:"id"`
	Year       int     `json:"year"`
	Quarter    int     `json:"quarter"`
	Type       string  `json:"decl_type"`
	TotalSales float64 `json:"total_sales"`
	VATOut     float64 `json:"total_vat_out"`
	TotalPurch float64 `json:"total_purchases"`
	VATIn      float64 `json:"total_vat_in"`
	VATDue     float64 `json:"vat_due"`
	Status     string  `json:"status"`
}

// DayRevenue — выручка за день.
type DayRevenue struct {
	Day   string  `json:"day"`
	Total float64 `json:"total"`
}

// TopProduct — топ товар по выручке.
type TopProduct struct {
	SKU   string  `json:"sku"`
	Name  string  `json:"name"`
	Qty   float64 `json:"qty"`
	Total float64 `json:"total"`
}

// TaxSummary — сводка дашборда.
type TaxSummary struct {
	Revenue30  float64      `json:"revenue_30d"`
	Receipts30 int          `json:"receipts_30d"`
	VATOut30   float64      `json:"vat_out_30d"`
	ByDay      []DayRevenue `json:"by_day"`
	Top        []TopProduct `json:"top_products"`
}
