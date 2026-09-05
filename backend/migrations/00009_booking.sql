-- Этап 8: услуги и бронирование (по sql/05). Отличия: timestamptz, employee → users
-- (таблицы employee нет), end_datetime считает Go (без триггера), без plpgsql-функций
-- и матпредставления (логика в Go).

-- Услуги — расширение catalog_product.
ALTER TABLE catalog_product ADD COLUMN IF NOT EXISTS product_type VARCHAR(20) DEFAULT 'GOODS'
    CHECK (product_type IN ('GOODS','SERVICE'));
ALTER TABLE catalog_product ADD COLUMN IF NOT EXISTS service_duration_minutes INT;
ALTER TABLE catalog_product ADD COLUMN IF NOT EXISTS service_requires_booking BOOLEAN DEFAULT FALSE;
ALTER TABLE catalog_product ADD COLUMN IF NOT EXISTS service_allow_walk_in BOOLEAN DEFAULT TRUE;
ALTER TABLE catalog_product ADD COLUMN IF NOT EXISTS service_booking_enabled BOOLEAN DEFAULT TRUE;
CREATE INDEX IF NOT EXISTS idx_product_type_service ON catalog_product(product_type) WHERE product_type = 'SERVICE';

CREATE TABLE IF NOT EXISTS service_resource_type (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL
);
INSERT INTO service_resource_type (code, name) VALUES
('ROOM', 'Помещение'), ('EQUIPMENT', 'Оборудование'), ('EMPLOYEE', 'Сотрудник')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS service_resource (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    resource_type_code VARCHAR(20) NOT NULL REFERENCES service_resource_type(code),
    name VARCHAR(100) NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    location VARCHAR(255),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_resource_org ON service_resource(organization_id);

CREATE TABLE IF NOT EXISTS service_product_resource (
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,
    is_mandatory BOOLEAN DEFAULT TRUE,
    PRIMARY KEY (product_id, resource_id)
);

CREATE TABLE IF NOT EXISTS service_booking_status (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_terminal BOOLEAN DEFAULT FALSE
);
INSERT INTO service_booking_status (code, name, is_terminal) VALUES
('PENDING', 'Ожидает подтверждения', FALSE),
('CONFIRMED', 'Подтверждено', FALSE),
('IN_PROGRESS', 'Выполняется', FALSE),
('COMPLETED', 'Выполнено', TRUE),
('CANCELED', 'Отменено', TRUE),
('NO_SHOW', 'Клиент не явился', TRUE)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS service_booking (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    customer_id BIGINT REFERENCES counterparty(id) ON DELETE SET NULL,
    customer_name VARCHAR(255),
    customer_phone VARCHAR(20),
    customer_email VARCHAR(100),
    start_datetime TIMESTAMPTZ NOT NULL,
    end_datetime TIMESTAMPTZ NOT NULL,
    duration_minutes INT NOT NULL CHECK (duration_minutes > 0),
    status_code VARCHAR(20) NOT NULL DEFAULT 'PENDING' REFERENCES service_booking_status(code),
    notes TEXT,
    sales_receipt_item_id BIGINT REFERENCES sales_receipt_item(id) ON DELETE SET NULL,
    sales_order_line_id BIGINT REFERENCES sales_order_line(id) ON DELETE SET NULL,
    sales_order_id BIGINT REFERENCES sales_order(id) ON DELETE SET NULL,
    created_by_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ,
    notification_sent BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_booking_org_start ON service_booking(organization_id, start_datetime);
CREATE INDEX IF NOT EXISTS idx_booking_status ON service_booking(status_code);

CREATE TABLE IF NOT EXISTS service_booking_item (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES service_booking(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    quantity INT NOT NULL DEFAULT 1 CHECK (quantity > 0),
    price DECIMAL(15,2) NOT NULL CHECK (price >= 0),
    duration_minutes INT,
    sales_receipt_item_id BIGINT REFERENCES sales_receipt_item(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS service_booking_resource (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES service_booking(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,
    booking_item_id BIGINT REFERENCES service_booking_item(id) ON DELETE CASCADE,
    is_confirmed BOOLEAN DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_booking_res_resource ON service_booking_resource(resource_id);

CREATE TABLE IF NOT EXISTS service_booking_status_history (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES service_booking(id) ON DELETE CASCADE,
    status_code VARCHAR(20) NOT NULL REFERENCES service_booking_status(code),
    changed_by_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    comment TEXT,
    changed_at TIMESTAMPTZ DEFAULT NOW()
);

-- Расписание ресурсов.
CREATE TABLE IF NOT EXISTS service_resource_schedule (
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,
    day_of_week INT NOT NULL CHECK (day_of_week BETWEEN 1 AND 7),
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    PRIMARY KEY (resource_id, day_of_week)
);

CREATE TABLE IF NOT EXISTS service_resource_schedule_exception (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,
    exception_date DATE NOT NULL,
    is_working BOOLEAN DEFAULT FALSE,
    start_time TIME,
    end_time TIME,
    reason TEXT,
    UNIQUE (resource_id, exception_date)
);

-- Шаблоны напоминаний (типы BOOKING_* уже в 00008).
INSERT INTO notification_template (notification_type_code, channel_code, name, subject, body_template) VALUES
('BOOKING_CREATED', 'EMAIL', 'Бронирование создано', 'Бронирование №{{booking_id}} создано',
 'Здравствуйте! Бронирование №{{booking_id}} ({{service_name}}, {{start_datetime}}) создано, ожидайте подтверждения.'),
('BOOKING_CREATED', 'WEB', 'Бронирование создано (WEB)', NULL,
 'Новое бронирование №{{booking_id}}: {{service_name}}, {{start_datetime}}.'),
('BOOKING_CONFIRMED', 'EMAIL', 'Бронирование подтверждено (письмо)', 'Бронирование №{{booking_id}} подтверждено',
 'Здравствуйте! Ваше бронирование на {{service_name}} {{start_datetime}} подтверждено.'),
('BOOKING_CONFIRMED', 'WEB', 'Бронирование подтверждено (WEB2)', NULL,
 'Бронирование №{{booking_id}} подтверждено: {{service_name}}, {{start_datetime}}.'),
('BOOKING_REMINDER', 'EMAIL', 'Напоминание (письмо)', 'Напоминание о бронировании №{{booking_id}}',
 'Напоминаем: {{service_name}} {{start_datetime}} (бронирование №{{booking_id}}).'),
('BOOKING_REMINDER', 'WEB', 'Напоминание (WEB)', NULL,
 'Скоро: {{service_name}} {{start_datetime}} (№{{booking_id}}).')
ON CONFLICT (notification_type_code, channel_code) DO NOTHING;

-- Обновляем шаблон из 00008 под переменные, которые реально шлет бэкенд.
UPDATE notification_template
SET subject = 'Бронирование №{{booking_id}} подтверждено',
    body_template = 'Здравствуйте! Ваше бронирование на {{service_name}} {{start_datetime}} подтверждено.'
WHERE notification_type_code = 'BOOKING_CONFIRMED' AND channel_code = 'EMAIL';
