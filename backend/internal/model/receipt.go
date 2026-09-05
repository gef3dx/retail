package model

import "math"

// Register — касса ККТ.
type Register struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	RegNumber      string `json:"reg_number"`
	Model          string `json:"model"`
	Status         string `json:"status"`
	WarehouseID    *int64 `json:"warehouse_id,omitempty"`
}

// Shift — кассовая смена.
type Shift struct {
	ID          int64  `json:"id"`
	ShiftNumber int64  `json:"shift_number"`
	RegisterID  int64  `json:"cash_register_id"`
	Status      string `json:"status"`
}

// ShiftReport — X/Z-отчёт смены.
type ShiftReport struct {
	ShiftID      int64   `json:"shift_id"`
	ShiftNumber  int64   `json:"shift_number"`
	SaleCount    int     `json:"sale_count"`
	ReturnCount  int     `json:"return_count"`
	CashSales    float64 `json:"cash_sales"`
	CardSales    float64 `json:"card_sales"`
	CashReturns  float64 `json:"cash_returns"`
	CardReturns  float64 `json:"card_returns"`
	StartCash    float64 `json:"start_cash"`
	ExpectedCash float64 `json:"expected_cash"`
}

// Receipt — чек (плоский вид для списков).
type Receipt struct {
	ID            int64   `json:"id"`
	ReceiptNumber string  `json:"receipt_number"`
	ReceiptType   string  `json:"receipt_type"`
	TotalAmount   float64 `json:"total_amount"`
	PaymentType   string  `json:"payment_type"`
	CreatedAt     string  `json:"created_at"`
	OFDStatus     string  `json:"ofd_status"`
	FiscalSign    string  `json:"fiscal_sign,omitempty"`
}

// ReceiptLine — позиция чека в доменных терминах.
type ReceiptLine struct {
	ProductID int64
	Name      string
	SKU       string
	Qty       float64
	Price     float64
	VAT       float64
	Discount  float64
	Marked    bool
	Attr      string
	Method    string
	CodeIDs   []int64
	BookingID *int64
}

func Round2(x float64) float64 { return math.Round(x*100) / 100 }

// LineTotals считает итоги позиций: сумма, НДС, флаг маркировки.
func LineTotals(lines []ReceiptLine) (total, vat float64, marked bool) {
	for _, l := range lines {
		t := Round2(l.Price*l.Qty - l.Discount)
		total += t
		vat += Round2(t * l.VAT / 100)
		if l.Marked {
			marked = true
		}
	}
	return Round2(total), Round2(vat), marked
}

// LineQty сворачивает позиции в карту product → qty.
func LineQty(lines []ReceiptLine) map[int64]float64 {
	m := map[int64]float64{}
	for _, l := range lines {
		m[l.ProductID] += l.Qty
	}
	return m
}

// FiscalItem — позиция чека для фискализации (ФФД 1.2, упрощённо).
type FiscalItem struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Price    float64 `json:"price"`
	VATRate  float64 `json:"vat_rate"`
}

// FiscalPayload — чек для отправки провайдеру фискализации.
type FiscalPayload struct {
	ReceiptID int64        `json:"receipt_id"`
	Number    string       `json:"receipt_number"`
	Type      string       `json:"receipt_type"`
	Total     float64      `json:"total"`
	Cash      float64      `json:"payment_cash"`
	Card      float64      `json:"payment_card"`
	Items     []FiscalItem `json:"items"`
}

// FiscalResult — фискальные данные от провайдера.
type FiscalResult struct {
	DocNumber string `json:"fiscal_document_number"`
	Sign      string `json:"fiscal_sign"`
	QRURL     string `json:"qr_url"`
}
