-- Этап 11: уведомления без моков. Адреса получателей.
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_chat_id VARCHAR(32);
ALTER TABLE users ADD COLUMN IF NOT EXISTS push_token VARCHAR(255);
