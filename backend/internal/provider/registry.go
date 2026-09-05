package provider

// Registry — все известные провайдеры.
type Registry struct {
	providers []Provider
	byCode    map[string]Provider
}

// DefaultRegistry регистрирует эмуляторы (по умолчанию) и скелеты реальных
// провайдеров (неактивны без ключей, реал — этапы 11-15).
func DefaultRegistry() *Registry {
	r := &Registry{byCode: map[string]Provider{}}
	r.add(base{code: "OFD_EMULATOR", name: "ОФД: эмулятор ККТ", kind: KindOFD, emulator: true})
	r.add(base{code: "OFD_HTTP", name: "ОФД: HTTP-адаптер", kind: KindOFD, keys: []KeySpec{
		{Key: "api_url", Name: "API URL ОФД", Required: true},
		{Key: "api_key", Name: "API-ключ", Secret: true, Required: true},
		{Key: "inn", Name: "ИНН организации"},
	}})
	r.add(base{code: "GISMT_EMULATOR", name: "ГИС МТ: эмулятор", kind: KindGISMT, emulator: true})
	r.add(base{code: "GISMT_TRUEAPI", name: "Честный знак: True API", kind: KindGISMT, keys: []KeySpec{
		{Key: "token", Name: "Токен ОИС", Secret: true, Required: true},
		{Key: "oms_id", Name: "ID ОИС"},
	}})
	r.add(base{code: "EMAIL_SMTP", name: "Email: SMTP", kind: KindNotify, keys: []KeySpec{
		{Key: "host", Name: "SMTP host", Required: true},
		{Key: "port", Name: "Порт", Required: true},
		{Key: "username", Name: "Логин"},
		{Key: "password", Name: "Пароль", Secret: true},
		{Key: "from", Name: "От кого", Required: true},
		{Key: "tls", Name: "TLS (true/false)"},
	}})
	r.add(base{code: "TELEGRAM_BOT", name: "Telegram: Bot API", kind: KindNotify, keys: []KeySpec{
		{Key: "bot_token", Name: "Токен бота", Secret: true, Required: true},
	}})
	r.add(base{code: "SMS_PROVIDER", name: "SMS: провайдер", kind: KindNotify, keys: []KeySpec{
		{Key: "api_url", Name: "API URL", Required: true},
		{Key: "api_key", Name: "API-ключ", Secret: true, Required: true},
		{Key: "sender", Name: "Подпись отправителя"},
	}})
	r.add(base{code: "PUSH_PROVIDER", name: "Push: провайдер", kind: KindNotify, keys: []KeySpec{
		{Key: "api_key", Name: "API-ключ", Secret: true, Required: true},
	}})
	r.add(base{code: "WHATSAPP_GENERIC", name: "WhatsApp: HTTP-шлюз", kind: KindNotify, keys: []KeySpec{
		{Key: "api_url", Name: "API URL", Required: true},
		{Key: "api_key", Name: "API-ключ", Secret: true},
	}})
	r.add(base{code: "DELIVERY_EMULATOR", name: "Доставка: своя (эмулятор служб)", kind: KindDelivery, emulator: true})
	r.add(base{code: "DELIVERY_CDEK", name: "Доставка: СДЭК", kind: KindDelivery, keys: []KeySpec{
		{Key: "api_url", Name: "API URL", Required: true},
		{Key: "client_id", Name: "Client ID", Required: true},
		{Key: "client_secret", Name: "Client Secret", Secret: true, Required: true},
	}})
	r.add(base{code: "MARKET_OZON", name: "Маркетплейс: Ozon", kind: KindMarket, keys: []KeySpec{
		{Key: "client_id", Name: "Client ID", Required: true},
		{Key: "api_key", Name: "API-ключ", Secret: true, Required: true},
	}})
	r.add(base{code: "MARKET_WB", name: "Маркетплейс: Wildberries", kind: KindMarket, keys: []KeySpec{
		{Key: "api_key", Name: "API-ключ", Secret: true, Required: true},
	}})
	r.add(base{code: "MARKET_YANDEX", name: "Маркетплейс: Яндекс.Маркет", kind: KindMarket, keys: []KeySpec{
		{Key: "client_id", Name: "Client ID", Required: true},
		{Key: "api_key", Name: "API-ключ", Secret: true, Required: true},
		{Key: "campaign_id", Name: "ID кампании", Required: true},
	}})
	r.add(base{code: "EGAIS_UTM", name: "ЕГАИС: УТМ", kind: KindEGAIS, keys: []KeySpec{
		{Key: "utm_url", Name: "URL УТМ", Required: true},
		{Key: "fsrar_id", Name: "FSRAR ID"},
	}})
	return r
}

func (r *Registry) add(p Provider) {
	r.providers = append(r.providers, p)
	r.byCode[p.Code()] = p
}

// All — все провайдеры по порядку регистрации.
func (r *Registry) All() []Provider { return r.providers }

// ByCode — поиск по коду (nil если нет).
func (r *Registry) ByCode(code string) Provider { return r.byCode[code] }

// ActiveProvider возвращает код активного провайдера домена:
// первый ACTIVE (не эмулятор в приоритете), иначе эмулятор если включён.
func (r *Registry) ActiveFor(k Kind, statuses []ProviderStatus) string {
	var emulator string
	for _, s := range statuses {
		if s.Kind != k {
			continue
		}
		if s.Status == StatusActive {
			if !s.Emulator {
				return s.Code
			}
			emulator = s.Code
		}
	}
	return emulator
}

// ProviderStatus — вычисленный статус для организации.
type ProviderStatus struct {
	Code     string          `json:"code"`
	Name     string          `json:"name"`
	Kind     Kind            `json:"kind"`
	Status   Status          `json:"status"`
	Enabled  bool            `json:"enabled"`
	Emulator bool            `json:"emulator"`
	Keys     []KeySpec       `json:"keys"`
	HasValue map[string]bool `json:"has_value"`
	Missing  []string        `json:"missing"`
}
