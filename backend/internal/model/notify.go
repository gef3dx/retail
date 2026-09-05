package model

// Recipient — получатель уведомления.
type Recipient struct {
	UserID   int64  `json:"user_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Telegram string `json:"telegram,omitempty"`
	Push     string `json:"push,omitempty"`
}

// InboxItem — входящее WEB-уведомление.
type InboxItem struct {
	ID       int64   `json:"id"`
	Type     string  `json:"type"`
	Subject  *string `json:"subject,omitempty"`
	Body     *string `json:"body,omitempty"`
	Entity   *string `json:"entity,omitempty"`
	EntityID *int64  `json:"entity_id,omitempty"`
	Status   string  `json:"status"`
	At       string  `json:"at"`
}

// QueuedItem — строка очереди на чтение.
type QueuedItem struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Channel   string  `json:"channel"`
	To        *string `json:"to,omitempty"`
	Status    string  `json:"status"`
	Attempts  int     `json:"attempts"`
	Scheduled string  `json:"scheduled"`
	Priority  int     `json:"priority"`
	Entity    *string `json:"entity,omitempty"`
	EntityID  *int64  `json:"entity_id,omitempty"`
}

// Template — шаблон уведомления (превью).
type Template struct {
	ID      int64   `json:"id"`
	Type    string  `json:"type"`
	Channel string  `json:"channel"`
	Name    string  `json:"name"`
	Subject *string `json:"subject,omitempty"`
	Preview *string `json:"preview,omitempty"`
	Active  bool    `json:"active"`
}

// Preference — предпочтение пользователя.
type Preference struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	Enabled bool   `json:"enabled"`
}

// NotifySettings — настройки уведомлений организации.
type NotifySettings struct {
	Enabled           bool    `json:"enabled"`
	MaxAttempts       int     `json:"max_attempts"`
	FailFirstAttempts int     `json:"fail_first_attempts"`
	LowStockThreshold float64 `json:"low_stock_threshold"`
}
