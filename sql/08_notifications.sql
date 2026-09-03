-- ============================================================
-- 08_notifications.sql – Модуль уведомлений
-- ============================================================

-- ============================================================
-- 1. ТИПЫ УВЕДОМЛЕНИЙ (СПРАВОЧНИК)
-- ============================================================

CREATE TABLE notification_type (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE
);

COMMENT ON TABLE notification_type IS 'Типы уведомлений (заказ, доставка, бронирование и т.д.)';

INSERT INTO notification_type (code, name, description) VALUES
('ORDER_CREATED', 'Создан заказ', 'Уведомление о создании нового заказа'),
('ORDER_STATUS_CHANGED', 'Статус заказа изменён', 'Уведомление об изменении статуса заказа'),
('ORDER_PAID', 'Заказ оплачен', 'Уведомление об оплате заказа'),
('DELIVERY_CREATED', 'Создана доставка', 'Уведомление о создании доставки'),
('DELIVERY_STATUS_CHANGED', 'Статус доставки изменён', 'Уведомление об изменении статуса доставки'),
('DELIVERY_ASSIGNED', 'Курьер назначен', 'Уведомление о назначении курьера'),
('BOOKING_CREATED', 'Создано бронирование', 'Уведомление о новом бронировании'),
('BOOKING_CONFIRMED', 'Бронирование подтверждено', 'Уведомление о подтверждении бронирования'),
('BOOKING_REMINDER', 'Напоминание о бронировании', 'Напоминание за день до бронирования'),
('PAYMENT_RECEIVED', 'Получен платёж', 'Уведомление о поступлении платежа'),
('PAYMENT_FAILED', 'Платёж отклонён', 'Уведомление об ошибке оплаты'),
('STOCK_LOW', 'Товар заканчивается', 'Уведомление о низком остатке товара'),
('STOCK_ARRIVED', 'Товар поступил', 'Уведомление о поступлении товара'),
('MARKING_EXPIRY', 'Срок годности маркировки', 'Уведомление о скором истечении срока кода маркировки'),
('PROMOTION', 'Акция', 'Уведомление о новой акции'),
('SYSTEM_ALERT', 'Системное оповещение', 'Техническое уведомление');

-- ============================================================
-- 2. КАНАЛЫ ДОСТАВКИ (СПРАВОЧНИК)
-- ============================================================

CREATE TABLE notification_channel (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE
);

INSERT INTO notification_channel (code, name, description) VALUES
('EMAIL', 'Электронная почта', 'Отправка на email'),
('SMS', 'СМС', 'Отправка SMS-сообщений'),
('PUSH', 'Push-уведомление', 'Мобильное push-уведомление'),
('TELEGRAM', 'Telegram', 'Уведомление через Telegram-бота'),
('WEB', 'Веб-интерфейс', 'Уведомление в личном кабинете'),
('WHATSAPP', 'WhatsApp', 'Уведомление через WhatsApp');

-- ============================================================
-- 3. ШАБЛОНЫ УВЕДОМЛЕНИЙ
-- ============================================================

CREATE TABLE notification_template (
    id BIGSERIAL PRIMARY KEY,
    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),

    name VARCHAR(100) NOT NULL,
    subject VARCHAR(255), -- для email/SMS заголовок

    -- Содержимое (с поддержкой переменных {{variable}})
    body_template TEXT NOT NULL,
    html_body_template TEXT, -- для email HTML

    -- Дополнительные параметры (JSON)
    params JSONB DEFAULT '{}'::JSONB,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (notification_type_code, channel_code)
);

COMMENT ON TABLE notification_template IS 'Шаблоны уведомлений с переменными';

-- Вставим базовые шаблоны (примеры)
INSERT INTO notification_template (notification_type_code, channel_code, name, subject, body_template, html_body_template)
VALUES
('ORDER_CREATED', 'EMAIL', 'Заказ создан', 'Ваш заказ №{{order_number}} создан',
 'Здравствуйте, {{customer_name}}! Ваш заказ №{{order_number}} от {{order_date}} успешно создан. Сумма заказа: {{total_amount}} руб.',
 '<p>Здравствуйте, {{customer_name}}!</p><p>Ваш заказ №{{order_number}} от {{order_date}} успешно создан.</p><p>Сумма заказа: {{total_amount}} руб.</p>'),

('ORDER_CREATED', 'SMS', 'Заказ создан (SMS)', NULL,
 'Заказ №{{order_number}} создан. Сумма {{total_amount}} руб.', NULL),

('BOOKING_CONFIRMED', 'EMAIL', 'Бронирование подтверждено', 'Бронирование №{{booking_id}} подтверждено',
 'Здравствуйте, {{customer_name}}! Ваше бронирование на {{service_name}} {{start_datetime}} подтверждено.', NULL),

('DELIVERY_STATUS_CHANGED', 'EMAIL', 'Статус доставки изменён', 'Статус доставки заказа №{{order_number}} изменён',
 'Статус доставки вашего заказа №{{order_number}} обновлён: {{new_status}}.', NULL);

-- ============================================================
-- 4. ОЧЕРЕДЬ УВЕДОМЛЕНИЙ
-- ============================================================

CREATE TABLE notification_queue (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Тип и канал
    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),

    -- Кому отправляем
    recipient_name VARCHAR(255),
    recipient_email VARCHAR(100),
    recipient_phone VARCHAR(20),
    recipient_user_id BIGINT REFERENCES users(id),

    -- Содержание (может быть сгенерировано из шаблона или передано явно)
    subject TEXT,
    body TEXT,
    html_body TEXT,

    -- Данные для шаблона (JSON с переменными)
    template_data JSONB,

    -- Ссылка на сущность, вызвавшую уведомление
    entity_type VARCHAR(50), -- order, delivery, booking, etc.
    entity_id BIGINT,

    -- Статус отправки
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SENT', 'DELIVERED', 'FAILED', 'RETRY')),

    -- Попытки
    attempt_count INT DEFAULT 0,
    max_attempts INT DEFAULT 3,
    last_attempt_at TIMESTAMP,
    error_message TEXT,

    -- Запланированное время отправки (можно отложить)
    scheduled_at TIMESTAMP DEFAULT NOW(),
    sent_at TIMESTAMP,

    -- Приоритет (1-10, где 10 - самый высокий)
    priority INT DEFAULT 5,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE notification_queue IS 'Очередь уведомлений для отправки';

CREATE INDEX idx_notification_queue_status ON notification_queue(status) WHERE status = 'PENDING';
CREATE INDEX idx_notification_queue_scheduled ON notification_queue(scheduled_at);
CREATE INDEX idx_notification_queue_entity ON notification_queue(entity_type, entity_id);
CREATE INDEX idx_notification_queue_user ON notification_queue(recipient_user_id);

-- ============================================================
-- 5. ИСТОРИЯ ОТПРАВЛЕННЫХ УВЕДОМЛЕНИЙ
-- ============================================================

CREATE TABLE notification_history (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Копия данных из очереди после отправки
    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),

    recipient_name VARCHAR(255),
    recipient_email VARCHAR(100),
    recipient_phone VARCHAR(20),
    recipient_user_id BIGINT REFERENCES users(id),

    subject TEXT,
    body TEXT,
    html_body TEXT,

    entity_type VARCHAR(50),
    entity_id BIGINT,

    -- Статус отправки (успех/ошибка)
    status VARCHAR(20) CHECK (status IN ('SENT', 'FAILED', 'DELIVERED', 'VIEWED')),
    error_message TEXT,

    -- Внешний ID провайдера (например, ID письма в сервисе рассылки)
    provider_message_id VARCHAR(255),

    -- Даты
    sent_at TIMESTAMP DEFAULT NOW(),
    delivered_at TIMESTAMP,
    viewed_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE notification_history IS 'История отправленных уведомлений';

CREATE INDEX idx_notification_history_user ON notification_history(recipient_user_id);
CREATE INDEX idx_notification_history_entity ON notification_history(entity_type, entity_id);
CREATE INDEX idx_notification_history_sent ON notification_history(sent_at);
CREATE INDEX idx_notification_history_provider ON notification_history(provider_message_id);

-- ============================================================
-- 6. НАСТРОЙКИ ПРЕДПОЧТЕНИЙ ПОЛЬЗОВАТЕЛЕЙ
-- ============================================================

CREATE TABLE notification_preference (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    notification_type_code VARCHAR(50) NOT NULL REFERENCES notification_type(code),
    channel_code VARCHAR(20) NOT NULL REFERENCES notification_channel(code),

    -- Разрешить отправку (вкл/выкл)
    enabled BOOLEAN DEFAULT TRUE,

    -- Доп. параметры (например, только для важных)
    params JSONB DEFAULT '{}'::JSONB,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (user_id, notification_type_code, channel_code)
);

COMMENT ON TABLE notification_preference IS 'Настройки уведомлений для пользователей';

CREATE INDEX idx_notification_pref_user ON notification_preference(user_id);

-- ============================================================
-- 7. ИНДЕКСЫ
-- ============================================================

CREATE INDEX idx_notification_queue_status ON notification_queue(status) WHERE status = 'PENDING';
CREATE INDEX idx_notification_queue_scheduled ON notification_queue(scheduled_at);
CREATE INDEX idx_notification_queue_entity ON notification_queue(entity_type, entity_id);
CREATE INDEX idx_notification_queue_user ON notification_queue(recipient_user_id);

CREATE INDEX idx_notification_history_user ON notification_history(recipient_user_id);
CREATE INDEX idx_notification_history_entity ON notification_history(entity_type, entity_id);
CREATE INDEX idx_notification_history_sent ON notification_history(sent_at);

CREATE INDEX idx_notification_pref_user ON notification_preference(user_id);

-- ============================================================
-- 8. ТРИГГЕРЫ ДЛЯ ОБНОВЛЕНИЯ UPDATED_AT
-- ============================================================

DO $$
DECLARE
    table_name text;
BEGIN
    FOR table_name IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename IN ('notification_template', 'notification_queue', 'notification_preference')
    LOOP
        EXECUTE format('
            CREATE TRIGGER update_%I_updated_at
            BEFORE UPDATE ON %I
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column();
        ', table_name, table_name);
    END LOOP;
END;
$$;

-- ============================================================
-- 9. ФУНКЦИЯ ДЛЯ ПОСТАНОВКИ УВЕДОМЛЕНИЯ В ОЧЕРЕДЬ
-- ============================================================

CREATE OR REPLACE FUNCTION queue_notification(
    p_organization_id BIGINT,
    p_notification_type_code VARCHAR,
    p_channel_code VARCHAR,
    p_recipient_name VARCHAR,
    p_recipient_email VARCHAR,
    p_recipient_phone VARCHAR,
    p_recipient_user_id BIGINT,
    p_subject TEXT,
    p_body TEXT,
    p_html_body TEXT,
    p_template_data JSONB,
    p_entity_type VARCHAR,
    p_entity_id BIGINT,
    p_scheduled_at TIMESTAMP DEFAULT NOW(),
    p_priority INT DEFAULT 5
) RETURNS BIGINT AS $$
DECLARE
    v_queue_id BIGINT;
    v_template notification_template%ROWTYPE;
    v_body TEXT;
    v_html TEXT;
    v_subject TEXT;
BEGIN
    -- Если тело не передано, пытаемся использовать шаблон
    IF p_body IS NULL AND p_template_data IS NOT NULL THEN
        SELECT * INTO v_template
        FROM notification_template
        WHERE notification_type_code = p_notification_type_code
        AND channel_code = p_channel_code
        AND is_active = TRUE
        LIMIT 1;

        IF FOUND THEN
            -- Подставляем переменные в шаблон
            -- (в реальной реализации используется функция замены, здесь упрощённо)
            v_body := v_template.body_template;
            v_html := v_template.html_body_template;
            v_subject := v_template.subject;

            -- Здесь можно применить замену {{variable}} из template_data
            -- Для простоты пропустим, но в реальном проекте нужно реализовать.
        END IF;
    ELSE
        v_body := p_body;
        v_html := p_html_body;
        v_subject := p_subject;
    END IF;

    -- Вставляем в очередь
    INSERT INTO notification_queue (
        organization_id,
        notification_type_code,
        channel_code,
        recipient_name,
        recipient_email,
        recipient_phone,
        recipient_user_id,
        subject,
        body,
        html_body,
        template_data,
        entity_type,
        entity_id,
        scheduled_at,
        priority,
        status
    ) VALUES (
        p_organization_id,
        p_notification_type_code,
        p_channel_code,
        p_recipient_name,
        p_recipient_email,
        p_recipient_phone,
        p_recipient_user_id,
        COALESCE(v_subject, p_subject),
        COALESCE(v_body, p_body),
        COALESCE(v_html, p_html_body),
        p_template_data,
        p_entity_type,
        p_entity_id,
        p_scheduled_at,
        p_priority,
        'PENDING'
    ) RETURNING id INTO v_queue_id;

    RETURN v_queue_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION queue_notification IS 'Постановка уведомления в очередь отправки';

-- ============================================================
-- 10. ПРИМЕРЫ ИСПОЛЬЗОВАНИЯ
-- ============================================================

-- -- Постановка уведомления о создании заказа для клиента
-- SELECT queue_notification(
--     1, -- organization_id
--     'ORDER_CREATED', -- notification_type_code
--     'EMAIL', -- channel
--     'Иванов Иван',
--     'customer@example.com',
--     NULL,
--     NULL, -- recipient_user_id (если нет)
--     NULL, -- subject (используется шаблон)
--     NULL, -- body (используется шаблон)
--     NULL, -- html_body (используется шаблон)
--     '{"order_number":"12345","customer_name":"Иван","order_date":"2026-09-03","total_amount":"5000"}'::JSONB,
--     'order',
--     12345,
--     NOW()
-- );

-- -- Постановка SMS-уведомления
-- SELECT queue_notification(
--     1,
--     'ORDER_CREATED',
--     'SMS',
--     'Иванов Иван',
--     NULL,
--     '+79991234567',
--     NULL,
--     'Ваш заказ №12345 создан!',
--     NULL,
--     NULL,
--     NULL,
--     'order',
--     12345,
--     NOW()
-- );
