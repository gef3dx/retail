package model

import "time"

type User struct {
	ID            int64      `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	Phone         *string    `json:"phone,omitempty"`
	IsActive      bool       `json:"is_active"`
	Roles         []string   `json:"roles,omitempty"`
	Organizations []int64    `json:"organizations,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Organization struct {
	ID         int64     `json:"id"`
	INN        string    `json:"inn"`
	KPP        string    `json:"kpp"`
	FullName   string    `json:"full_name"`
	ShortName  *string   `json:"short_name,omitempty"`
	TaxSystem  string    `json:"tax_system"`
	Phone      *string   `json:"phone,omitempty"`
	Email      *string   `json:"email,omitempty"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

type Role struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}
