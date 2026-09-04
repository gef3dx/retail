-- Этап 4 fix: фискальный признак (ФП) — до 32 hex-символов в mock-провайдере,
-- в 00004 было VARCHAR(30). Расширяем с запасом.
ALTER TABLE ofd_send_status ALTER COLUMN fiscal_sign TYPE VARCHAR(64);
