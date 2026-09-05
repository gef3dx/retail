package model

import "time"

type Organization struct {
	ID        int64     `json:"id"`
	INN       string    `json:"inn"`
	KPP       string    `json:"kpp"`
	FullName  string    `json:"full_name"`
	ShortName *string   `json:"short_name,omitempty"`
	TaxSystem string    `json:"tax_system"`
	Phone     *string   `json:"phone,omitempty"`
	Email     *string   `json:"email,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}
