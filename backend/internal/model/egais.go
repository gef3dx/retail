package model

// EgaisDoc — документ ЕГАИС (outbox в УТМ).
type EgaisDoc struct {
	ID        int64   `json:"id"`
	Type      string  `json:"doc_type"`
	Status    string  `json:"status"`
	Reply     *string `json:"reply,omitempty"`
	Error     *string `json:"error_message,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// UtmStatus — состояние УТМ.
type UtmStatus struct {
	Reachable bool   `json:"reachable"`
	Version   string `json:"version,omitempty"`
	URL       string `json:"url,omitempty"`
	Error     string `json:"error,omitempty"`
}
