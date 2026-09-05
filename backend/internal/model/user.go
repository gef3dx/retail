package model

import "time"

// User — учетная запись. Роли/организации подгружаются отдельно.
type User struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	Phone         *string   `json:"phone,omitempty"`
	IsActive      bool      `json:"is_active"`
	Roles         []string  `json:"roles,omitempty"`
	Organizations []int64   `json:"organizations,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// UserCredentials — внутреннее: данные для проверки пароля и блокировок.
type UserCredentials struct {
	ID             int64
	Username       string
	PasswordHash   string
	IsActive       bool
	IsLocked       bool
	FailedAttempts int
	LockedUntil    *time.Time
}

type Role struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// Session — refresh-сессия (в БД хранится хеш токена).
type Session struct {
	ID               int64
	UserID           int64
	Username         string
	RefreshExpiresAt time.Time
	IsActive         bool
}
