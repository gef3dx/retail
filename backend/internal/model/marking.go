package model

// MarkingCode — код маркировки в пуле.
type MarkingCode struct {
	ID             int64  `json:"id"`
	Code           string `json:"code"`
	Status         string `json:"status"`
	ProductID      int64  `json:"product_id"`
	ProductName    string `json:"product_name"`
	BatchID        *int64 `json:"batch_id,omitempty"`
	SalesReceiptID *int64 `json:"sales_receipt_id,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// MarkingCheck — результат проверки кода.
type MarkingCheck struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Status      string `json:"status"`
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	CanSell     bool   `json:"can_sell"`
}

// GismtQueueItem — операция очереди ГИС МТ.
type GismtQueueItem struct {
	ID        int64   `json:"id"`
	Operation string  `json:"operation"`
	Status    string  `json:"status"`
	Attempts  int     `json:"attempts"`
	Code      string  `json:"code"`
	ReceiptID *int64  `json:"receipt_id,omitempty"`
	Error     *string `json:"error,omitempty"`
}

// IntegrationLogEntry — строка центрального лога интеграций.
type IntegrationLogEntry struct {
	ID         int64   `json:"id"`
	Type       string  `json:"type"`
	Direction  string  `json:"direction"`
	Endpoint   *string `json:"endpoint,omitempty"`
	IsError    bool    `json:"is_error"`
	ExternalID *string `json:"external_id,omitempty"`
	DocumentID *int64  `json:"document_id,omitempty"`
	At         string  `json:"at"`
}
