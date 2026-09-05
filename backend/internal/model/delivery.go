package model

// DeliveryZone — зона доставки с тарифом.
type DeliveryZone struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	BasePrice    float64  `json:"base_price"`
	PricePerKg   *float64 `json:"price_per_kg,omitempty"`
	FreeFrom     *float64 `json:"free_delivery_from,omitempty"`
	EstimatedMin *int     `json:"estimated_days_min,omitempty"`
	EstimatedMax *int     `json:"estimated_days_max,omitempty"`
	IsActive     bool     `json:"is_active"`
}

// Courier — курьер.
type Courier struct {
	ID          int64   `json:"id"`
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	Phone       string  `json:"phone"`
	Vehicle     *string `json:"vehicle_type,omitempty"`
	ZoneIDs     []int64 `json:"assigned_zone_ids,omitempty"`
	IsActive    bool    `json:"is_active"`
	IsAvailable bool    `json:"is_available"`
}

// DeliveryOrder — доставка (плоский вид).
type DeliveryOrder struct {
	ID          int64   `json:"id"`
	Type        string  `json:"delivery_type"`
	Address     string  `json:"address"`
	Recipient   string  `json:"recipient"`
	Price       float64 `json:"price"`
	Tracking    *string `json:"tracking_number,omitempty"`
	Status      string  `json:"status"`
	Courier     *string `json:"courier,omitempty"`
	DesiredDate *string `json:"desired_date,omitempty"`
}

// DeliveryHistory — запись истории статусов.
type DeliveryHistory struct {
	Status  string  `json:"status"`
	By      *int64  `json:"by,omitempty"`
	Comment *string `json:"comment,omitempty"`
	At      string  `json:"at"`
}

// DeliveryDetail — детали доставки.
type DeliveryDetail struct {
	ID          int64             `json:"id"`
	OrgID       int64             `json:"org_id"`
	SalesOrder  *int64            `json:"sales_order_id,omitempty"`
	Type        string            `json:"delivery_type"`
	Address     string            `json:"address"`
	Recipient   string            `json:"recipient"`
	Phone       string            `json:"phone"`
	ZoneID      *int64            `json:"zone_id,omitempty"`
	Price       float64           `json:"price"`
	Tracking    *string           `json:"tracking_number,omitempty"`
	Status      string            `json:"status"`
	DesiredDate *string           `json:"desired_date,omitempty"`
	Assignments []Assignment      `json:"assignments"`
	History     []DeliveryHistory `json:"history"`
}

// Assignment — назначение курьера.
type Assignment struct {
	ID        int64  `json:"id"`
	CourierID int64  `json:"courier_id"`
	Courier   string `json:"courier"`
	Status    string `json:"status"`
}
