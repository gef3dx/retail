package model

// Resource — ресурс для бронирования (сотрудник, кабинет, оборудование).
type Resource struct {
	ID       int64   `json:"id"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	UserID   *int64  `json:"user_id,omitempty"`
	Location *string `json:"location,omitempty"`
	IsActive bool    `json:"is_active"`
}

// SchedDay — день расписания ресурса.
type SchedDay struct {
	DOW    int    `json:"dow"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Active bool   `json:"active"`
}

// Booking — бронирование (плоский вид журнала).
type Booking struct {
	ID        int64  `json:"id"`
	Customer  string `json:"customer"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Status    string `json:"status"`
	Service   string `json:"service"`
	Resources string `json:"resources"`
}

// BookingHistory — запись истории статусов.
type BookingHistory struct {
	Status  string  `json:"status"`
	By      *int64  `json:"by,omitempty"`
	Comment *string `json:"comment,omitempty"`
	At      string  `json:"at"`
}

// BookingDetail — детали брони с историей.
type BookingDetail struct {
	ID       int64            `json:"id"`
	OrgID    int64            `json:"org_id"`
	Customer string           `json:"customer"`
	Phone    string           `json:"phone"`
	Email    string           `json:"email"`
	Notes    string           `json:"notes"`
	Status   string           `json:"status"`
	Start    string           `json:"start"`
	End      string           `json:"end"`
	Duration int              `json:"duration"`
	History  []BookingHistory `json:"history"`
}

// Slot — свободный интервал ресурса.
type Slot struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ProductResource — ресурс, привязанный к услуге.
type ProductResource struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Mandatory bool   `json:"mandatory"`
}
