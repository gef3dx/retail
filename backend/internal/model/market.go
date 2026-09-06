package model

// OfferLink — связка товара с оффером маркетплейса.
type OfferLink struct {
	ID        int64  `json:"id"`
	Provider  string `json:"provider_code"`
	ProductID int64  `json:"product_id"`
	OfferID   string `json:"offer_id"`
}

// MarketOrderItem — позиция заказа маркетплейса.
type MarketOrderItem struct {
	ID        int64   `json:"id"`
	ProductID *int64  `json:"product_id,omitempty"`
	OfferID   *string `json:"external_offer_id,omitempty"`
	Name      string  `json:"product_name"`
	Quantity  float64 `json:"quantity"`
	Price     float64 `json:"price"`
}

// MarketOrder — заказ с маркетплейса.
type MarketOrder struct {
	ID         int64             `json:"id"`
	Provider   string            `json:"provider_code"`
	ExternalID string            `json:"external_order_id"`
	Buyer      *string           `json:"buyer_name,omitempty"`
	Total      float64           `json:"total_amount"`
	Status     string            `json:"status"`
	MpStatus   *string           `json:"marketplace_status,omitempty"`
	SalesOrder *int64            `json:"sales_order_id,omitempty"`
	Error      *string           `json:"error_message,omitempty"`
	Items      []MarketOrderItem `json:"items,omitempty"`
}

// SyncLog — запись журнала синхронизаций.
type SyncLog struct {
	ID        int64   `json:"id"`
	Provider  string  `json:"provider_code"`
	Direction string  `json:"direction"`
	Operation string  `json:"operation"`
	Status    string  `json:"status"`
	Total     int     `json:"items_total"`
	OK        int     `json:"items_ok"`
	Error     *string `json:"error_message,omitempty"`
	At        string  `json:"created_at"`
}
