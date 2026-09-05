-- Этап 14: доставка (по sql/06). Отличия: timestamptz, employee → users,
-- без delivery_carrier (внешние службы — через провайдеров этапа 10),
-- без plpgsql-функций и матпредставления (логика в Go).

CREATE TABLE IF NOT EXISTS delivery_type (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL
);
INSERT INTO delivery_type (code, name) VALUES
('PICKUP', 'Самовывоз'),
('COURIER', 'Курьер'),
('POST', 'Почта России'),
('CDEK', 'СДЭК'),
('BOXBERY', 'Boxberry'),
('YANDEX_GO', 'Яндекс.Доставка'),
('OTHER', 'Другое')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS delivery_order_status (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_terminal BOOLEAN DEFAULT FALSE
);
INSERT INTO delivery_order_status (code, name, is_terminal) VALUES
('NEW', 'Новый', FALSE),
('ASSIGNED', 'Назначен', FALSE),
('PICKED_UP', 'Забран', FALSE),
('IN_TRANSIT', 'В пути', FALSE),
('ARRIVED', 'Прибыл', FALSE),
('DELIVERED', 'Доставлен', TRUE),
('CANCELED', 'Отменен', TRUE),
('RETURNED', 'Возвращен', TRUE)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS delivery_zone (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    base_price DECIMAL(15,2) NOT NULL DEFAULT 0,
    price_per_kg DECIMAL(10,2),
    free_delivery_from DECIMAL(15,2),
    estimated_days_min INT,
    estimated_days_max INT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS delivery_courier (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    first_name VARCHAR(50) NOT NULL,
    last_name VARCHAR(50) NOT NULL,
    phone VARCHAR(20) NOT NULL,
    vehicle_type VARCHAR(50),
    vehicle_number VARCHAR(20),
    assigned_zone_ids BIGINT[],
    is_active BOOLEAN DEFAULT TRUE,
    is_available BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_courier_org ON delivery_courier(organization_id);

CREATE TABLE IF NOT EXISTS delivery_order (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    sales_order_id BIGINT REFERENCES sales_order(id) ON DELETE SET NULL,
    delivery_type_code VARCHAR(20) NOT NULL REFERENCES delivery_type(code),
    delivery_address TEXT NOT NULL,
    recipient_name VARCHAR(255) NOT NULL,
    recipient_phone VARCHAR(20) NOT NULL,
    recipient_email VARCHAR(100),
    delivery_zone_id BIGINT REFERENCES delivery_zone(id) ON DELETE SET NULL,
    desired_delivery_date DATE,
    delivery_price DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_weight DECIMAL(10,3),
    tracking_number VARCHAR(100),
    status_code VARCHAR(20) NOT NULL DEFAULT 'NEW' REFERENCES delivery_order_status(code),
    created_by_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_delivery_org_status ON delivery_order(organization_id, status_code);
CREATE INDEX IF NOT EXISTS idx_delivery_sales ON delivery_order(sales_order_id);

CREATE TABLE IF NOT EXISTS delivery_order_status_history (
    id BIGSERIAL PRIMARY KEY,
    delivery_order_id BIGINT NOT NULL REFERENCES delivery_order(id) ON DELETE CASCADE,
    status_code VARCHAR(20) NOT NULL REFERENCES delivery_order_status(code),
    changed_by_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    comment TEXT,
    changed_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS delivery_order_assignment (
    id BIGSERIAL PRIMARY KEY,
    delivery_order_id BIGINT NOT NULL REFERENCES delivery_order(id) ON DELETE CASCADE,
    courier_id BIGINT NOT NULL REFERENCES delivery_courier(id) ON DELETE RESTRICT,
    assigned_by_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    assigned_at TIMESTAMPTZ DEFAULT NOW(),
    accepted_at TIMESTAMPTZ,
    status VARCHAR(20) DEFAULT 'ASSIGNED'
        CHECK (status IN ('ASSIGNED','ACCEPTED','REJECTED','COMPLETED','CANCELED')),
    pickup_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_assignment_order ON delivery_order_assignment(delivery_order_id);
CREATE INDEX IF NOT EXISTS idx_assignment_courier ON delivery_order_assignment(courier_id);

CREATE TABLE IF NOT EXISTS delivery_courier_schedule (
    courier_id BIGINT NOT NULL REFERENCES delivery_courier(id) ON DELETE CASCADE,
    work_date DATE NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    is_working BOOLEAN DEFAULT TRUE,
    PRIMARY KEY (courier_id, work_date)
);

-- Шаблоны уведомлений доставки (типы уже в 00008).
INSERT INTO notification_template (notification_type_code, channel_code, name, subject, body_template) VALUES
('DELIVERY_CREATED', 'WEB', 'Доставка создана (WEB)', NULL,
 'Новая доставка №{{delivery_id}} ({{delivery_type}}, {{address}}).'),
('DELIVERY_CREATED', 'EMAIL', 'Доставка создана', 'Доставка №{{delivery_id}} создана',
 'Здравствуйте! Доставка №{{delivery_id}} создана. Адрес: {{address}}.'),
('DELIVERY_STATUS_CHANGED', 'WEB', 'Статус доставки (WEB)', NULL,
 'Доставка №{{delivery_id}} → {{new_status}}.'),
('DELIVERY_STATUS_CHANGED', 'EMAIL', 'Статус доставки', 'Доставка №{{delivery_id}}: {{new_status}}',
 'Статус вашей доставки №{{delivery_id}} изменён: {{new_status}}.'),
('DELIVERY_ASSIGNED', 'WEB', 'Курьер назначен (WEB)', NULL,
 'На доставку №{{delivery_id}} назначен курьер {{courier_name}}.'),
('DELIVERY_ASSIGNED', 'EMAIL', 'Курьер назначен', 'Курьер назначен на доставку №{{delivery_id}}',
 'На вашу доставку №{{delivery_id}} назначен курьер {{courier_name}}, тел. {{courier_phone}}.')
ON CONFLICT (notification_type_code, channel_code) DO NOTHING;
