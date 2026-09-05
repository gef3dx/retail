-- Этап 13: ГИС МТ без моков. Строгий режим онлайн-проверки маркировки.
ALTER TABLE gismt_settings ADD COLUMN IF NOT EXISTS strict_online BOOLEAN DEFAULT FALSE;
