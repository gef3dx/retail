-- Этап 7: уведомления (по sql/08). Отличия: timestamptz вместо timestamp,
-- шаблонизация в Go (без plpgsql-функций и триггеров), + notify_settings для
-- mock-воркера и порога низких остатков, + тип RECEIPT_SOLD для связки с кассой.

CREATE TABLE IF NOT EXISTS notification_type (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE
);
INSERT INTO notification_type (code, name) VALUES
('ORDER_CREATED', 'Создан заказ'),
('ORDER_STATUS_CHANGED', 'Статус заказа изменён'),
('ORDER_PAID', 'Заказ оплачен'),
('DELIVERY_CREATED', 'Создана доставка'),
('DELIVERY_STATUS_CHANGED', 'Статус доставки изменён'),
('DELIVERY_ASSIGNED', 'Курьер назначен'),
('BOOKING_CREATED', 'Создано бронирование'),
('BOOKING_CONFIRMED', 'Бронирование подтверждено'),
('BOOKING_REMINDER', 'Напоминание о бронировании'),
('PAYMENT_RECEIVED', 'Получен платёж'),
('PAYMENT_FAILED', 'Платёж отклонён'),
('STOCK_LOW', 'Товар заканчивается'),
('STOCK_ARRIVED', 'Товар поступил'),
('MARKING_EXPIRY', 'Срок годности маркировки'),
('PROMOTION', 'Акция'),
('SYSTEM_ALERT', 'Системное оповещение'),
('RECEIPT_SOLD', 'Чек пробит')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS notification_channel (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE
);
INSERT INTO notification_channel (code, name) VALUES
('EMAIL', 'Электронная почта'),
('SMS', 'СМС'),
('PUSH', 'Push-уведомление'),
('TELEGRAM', 'Telegram'),
('WEB', 'Веб-интерфейс'),
('WHATSAPP', 'WhatsApp')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS notification_template (
    id BIGSERIAL PRIMARY KEY,
    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),
    name VARCHAR(100) NOT NULL,
    subject VARCHAR(255),
    body_template TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (notification_type_code, channel_code)
);
INSERT INTO notification_template (notification_type_code, channel_code, name, subject, body_template) VALUES
('ORDER_CREATED', 'EMAIL', 'Заказ создан', 'Заказ №{{order_number}} создан',
 'Здравствуйте! Заказ №{{order_number}} от {{order_date}} создан. Сумма: {{total_amount}} руб.'),
('ORDER_CREATED', 'SMS', 'Заказ создан (SMS)', NULL,
 'Заказ №{{order_number}} создан. Сумма {{total_amount}} руб.'),
('ORDER_CREATED', 'WEB', 'Заказ создан (WEB)', NULL,
 'Новый заказ №{{order_number}} на сумму {{total_amount}} руб.'),
('ORDER_STATUS_CHANGED', 'EMAIL', 'Статус заказа', 'Заказ №{{order_number}}: {{new_status}}',
 'Статус заказа №{{order_number}} изменён: {{new_status}}.'),
('ORDER_STATUS_CHANGED', 'WEB', 'Статус заказа (WEB)', NULL,
 'Заказ №{{order_number}} → {{new_status}}.'),
('STOCK_LOW', 'EMAIL', 'Низкий остаток', 'Товар заканчивается: {{product_name}}',
 'Остаток товара {{product_name}} ({{sku}}): {{available}} шт. на складе {{warehouse}}.'),
('STOCK_LOW', 'WEB', 'Низкий остаток (WEB)', NULL,
 'Низкий остаток: {{product_name}} — {{available}} шт. ({{warehouse}}).'),
('STOCK_ARRIVED', 'WEB', 'Поступление (WEB)', NULL,
 'Поступление {{doc_number}} проведено: +{{total_qty}} ед. на склад {{warehouse}}.'),
('RECEIPT_SOLD', 'WEB', 'Чек пробит (WEB)', NULL,
 'Чек №{{receipt_number}} на {{total_amount}} руб. ({{payment_type}}). ФД {{fiscal_doc}}.'),
('SYSTEM_ALERT', 'WEB', 'Системное (WEB)', NULL,
 '{{message}}')
ON CONFLICT (notification_type_code, channel_code) DO NOTHING;

CREATE TABLE IF NOT EXISTS notification_queue (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),
    recipient_name VARCHAR(255),
    recipient_email VARCHAR(100),
    recipient_phone VARCHAR(20),
    recipient_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    subject TEXT,
    body TEXT,
    template_data JSONB,
    entity_type VARCHAR(50),
    entity_id BIGINT,
    status VARCHAR(20) DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','SENT','DELIVERED','FAILED','RETRY')),
    attempt_count INT DEFAULT 0,
    max_attempts INT DEFAULT 3,
    last_attempt_at TIMESTAMPTZ,
    error_message TEXT,
    scheduled_at TIMESTAMPTZ DEFAULT NOW(),
    priority INT DEFAULT 5,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nq_pending ON notification_queue(status, scheduled_at)
    WHERE status IN ('PENDING','RETRY');
CREATE INDEX IF NOT EXISTS idx_nq_entity ON notification_queue(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_nq_user ON notification_queue(recipient_user_id);

CREATE TABLE IF NOT EXISTS notification_history (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),
    recipient_name VARCHAR(255),
    recipient_email VARCHAR(100),
    recipient_phone VARCHAR(20),
    recipient_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    subject TEXT,
    body TEXT,
    entity_type VARCHAR(50),
    entity_id BIGINT,
    status VARCHAR(20) CHECK (status IN ('SENT','FAILED','DELIVERED','VIEWED')),
    error_message TEXT,
    provider_message_id VARCHAR(255),
    sent_at TIMESTAMPTZ DEFAULT NOW(),
    viewed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_nh_user ON notification_history(recipient_user_id);
CREATE INDEX IF NOT EXISTS idx_nh_entity ON notification_history(entity_type, entity_id);

CREATE TABLE IF NOT EXISTS notification_preference (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),
    enabled BOOLEAN DEFAULT TRUE,
    PRIMARY KEY (user_id, notification_type_code, channel_code)
);

-- Настройки уведомлений организации (mock-воркер + порог остатков).
CREATE TABLE IF NOT EXISTS notify_settings (
    organization_id BIGINT PRIMARY KEY REFERENCES organization(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT TRUE,
    max_attempts INT DEFAULT 3,
    fail_first_attempts INT DEFAULT 0,
    low_stock_threshold DECIMAL(15,3) DEFAULT 10,
    is_active BOOLEAN DEFAULT TRUE
);
