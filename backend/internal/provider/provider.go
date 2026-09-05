package provider

// Kind — домен интеграции.
type Kind string

const (
	KindOFD      Kind = "OFD"
	KindGISMT    Kind = "GISMT"
	KindNotify   Kind = "NOTIFY"
	KindDelivery Kind = "DELIVERY"
	KindMarket   Kind = "MARKET"
	KindEGAIS    Kind = "EGAIS"
)

// Status — вычисляемый статус провайдера для организации.
type Status string

const (
	// ACTIVE — включён и настроен (ключи есть), функции доступны.
	StatusActive Status = "ACTIVE"
	// INACTIVE — нет ключей или провайдер не настроен, функции заблокированы.
	StatusInactive Status = "INACTIVE"
	// DISABLED — выключен вручную, функции заблокированы.
	StatusDisabled Status = "DISABLED"
)

// KeySpec — описание одного ключа/параметра.
type KeySpec struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

// Provider — описание провайдера интеграции.
type Provider interface {
	Code() string
	Name() string
	Kind() Kind
	// Keys — какие параметры нужны (порядок для UI).
	Keys() []KeySpec
	// IsConfigured — достаточно ли credentials для работы.
	IsConfigured(creds map[string]string) bool
	// Emulator — true для встроенного эмулятора (работает без ключей).
	Emulator() bool
	// Test — быстрая проверка соединения (эмулятор: всегда ok).
	Test(creds map[string]string) (bool, string)
}

type base struct {
	code     string
	name     string
	kind     Kind
	keys     []KeySpec
	emulator bool
}

func (b base) Code() string    { return b.code }
func (b base) Name() string    { return b.name }
func (b base) Kind() Kind      { return b.kind }
func (b base) Keys() []KeySpec { return b.keys }
func (b base) Emulator() bool  { return b.emulator }
func (b base) IsConfigured(creds map[string]string) bool {
	if b.emulator {
		return true
	}
	for _, k := range b.keys {
		if k.Required && creds[k.Key] == "" {
			return false
		}
	}
	return len(b.keys) > 0
}
func (b base) Test(creds map[string]string) (bool, string) {
	if b.emulator {
		return true, "emulator always ok"
	}
	if !b.IsConfigured(creds) {
		return false, "missing required keys"
	}
	return true, "credentials present (live check on next stages)"
}
