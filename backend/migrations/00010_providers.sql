-- Этап 10: фреймворк провайдеров. Единое хранилище credentials для всех
-- интеграций (ОФД, ГИС МТ, уведомления, доставка, маркетплейсы, ЕГАИС).
-- Секреты шифруются на уровне приложения (AES-GCM, ключ SETTINGS_ENC_KEY).

CREATE TABLE IF NOT EXISTS integration_settings (
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    provider_code VARCHAR(50) NOT NULL,
    credentials JSONB NOT NULL DEFAULT '{}'::JSONB,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (organization_id, provider_code)
);
