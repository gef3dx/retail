-- Этап 12: ОФД без моков. Ключ для HTTP-адаптера (секрет лучше держать
-- в integration_settings/OFD_HTTP; колонка — для совместимости и простых сетапов).
ALTER TABLE ofd_settings ADD COLUMN IF NOT EXISTS api_key VARCHAR(255);
