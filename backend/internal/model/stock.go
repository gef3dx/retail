package model

// Counterparty — контрагент (поставщик/покупатель).
type Counterparty struct {
	ID          int64   `json:"id"`
	INN         string  `json:"inn"`
	FullName    string  `json:"full_name"`
	Phone       *string `json:"phone,omitempty"`
	IsSupplier  bool    `json:"is_supplier"`
	IsBuyer     bool    `json:"is_buyer"`
	CreditLimit float64 `json:"credit_limit"`
}

// Warehouse — склад.
type Warehouse struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
	Type string `json:"warehouse_type"`
}

// Balance — остаток товара на складе.
type Balance struct {
	ProductID int64   `json:"product_id"`
	SKU       string  `json:"sku"`
	Name      string  `json:"name"`
	Quantity  float64 `json:"quantity"`
	Reserved  float64 `json:"reserved"`
	Available float64 `json:"available"`
}

// StockReceipt — поступление (шапка).
type StockReceipt struct {
	ID     int64   `json:"id"`
	Number string  `json:"number"`
	Date   string  `json:"date"`
	Total  float64 `json:"total"`
	Posted bool    `json:"posted"`
}

// Order — заказ покупателя (шапка).
type Order struct {
	ID     int64   `json:"id"`
	Number string  `json:"number"`
	Type   string  `json:"type"`
	Total  float64 `json:"total"`
	Status string  `json:"status"`
	Buyer  string  `json:"buyer"`
}

// OrderLine — строка заказа с отгруженным.
type OrderLine struct {
	ID        int64   `json:"id"`
	ProductID int64   `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  float64 `json:"quantity"`
	Price     float64 `json:"price"`
	Reserved  float64 `json:"reserved"`
	Shipped   float64 `json:"shipped"`
}

// OrderDetail — заказ с позициями.
type OrderDetail struct {
	ID          int64       `json:"id"`
	Number      string      `json:"number"`
	Status      string      `json:"status"`
	Total       float64     `json:"total"`
	WarehouseID int64       `json:"warehouse_id"`
	Lines       []OrderLine `json:"lines"`
}

// Shipment — отгрузка (шапка).
type Shipment struct {
	ID      int64   `json:"id"`
	Number  string  `json:"number"`
	Total   float64 `json:"total"`
	Posted  bool    `json:"posted"`
	OrderID *int64  `json:"order_id,omitempty"`
}
