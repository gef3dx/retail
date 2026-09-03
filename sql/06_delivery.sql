-- ============================================================
-- 06_delivery.sql – Модуль управления доставкой
-- ============================================================

-- ============================================================
-- 1. СПРАВОЧНИКИ ДОСТАВКИ
-- ============================================================

-- 1.1. Типы доставки
CREATE TABLE delivery_type (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE
);

COMMENT ON TABLE delivery_type IS 'Типы доставки (самовывоз, курьер, почта, СДЭК и т.д.)';

INSERT INTO delivery_type (code, name, description) VALUES
('PICKUP', 'Самовывоз', 'Клиент забирает заказ самостоятельно из магазина'),
('COURIER', 'Курьер', 'Доставка курьером до двери'),
('POST', 'Почта России', 'Отправка почтовым отправлением'),
('CDEK', 'СДЭК', 'Доставка службой СДЭК'),
('BOXBERY', 'Boxberry', 'Доставка службой Boxberry'),
('DHL', 'DHL', 'Международная доставка DHL'),
('YANDEX_GO', 'Яндекс.Доставка', 'Доставка через Яндекс.Go'),
('OTHER', 'Другое', 'Иной способ доставки');

-- 1.2. Статусы заказа на доставку
CREATE TABLE delivery_order_status (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_terminal BOOLEAN DEFAULT FALSE
);

INSERT INTO delivery_order_status (code, name, description, is_terminal) VALUES
('NEW', 'Новый', 'Заказ на доставку создан', FALSE),
('ASSIGNED', 'Назначен', 'Курьер назначен', FALSE),
('PICKED_UP', 'Забран', 'Заказ забран курьером', FALSE),
('IN_TRANSIT', 'В пути', 'Заказ в пути', FALSE),
('ARRIVED', 'Прибыл', 'Заказ прибыл в пункт выдачи/почту', FALSE),
('DELIVERED', 'Доставлен', 'Заказ доставлен получателю', TRUE),
('CANCELED', 'Отменен', 'Заказ отменен', TRUE),
('RETURNED', 'Возвращен', 'Заказ возвращен отправителю', TRUE);

-- 1.3. Курьерские службы
CREATE TABLE delivery_carrier (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(50) UNIQUE,
    description TEXT,

    -- Контактные данные
    phone VARCHAR(20),
    email VARCHAR(100),
    website VARCHAR(255),

    -- API настройки (для интеграции)
    api_url VARCHAR(255),
    api_key VARCHAR(255),
    api_login VARCHAR(100),
    api_password VARCHAR(255),

    -- Настройки
    tracking_url_template VARCHAR(255), -- шаблон для ссылки отслеживания
    is_active BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE delivery_carrier IS 'Курьерские службы и транспортные компании';

-- 1.4. Зоны доставки
CREATE TABLE delivery_zone (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    name VARCHAR(100) NOT NULL,
    description TEXT,

    -- Геоданные (полигон или список районов/городов)
    geo_data JSONB, -- { "type": "Polygon", "coordinates": [...] } или список районов

    -- Стоимость доставки
    base_price DECIMAL(15,2) NOT NULL DEFAULT 0,
    price_per_km DECIMAL(10,2),
    price_per_kg DECIMAL(10,2),
    free_delivery_from DECIMAL(15,2), -- бесплатная доставка от суммы заказа

    -- Сроки
    estimated_days_min INT,
    estimated_days_max INT,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE delivery_zone IS 'Зоны доставки с ценами и сроками';

-- ============================================================
-- 2. ЗАКАЗЫ НА ДОСТАВКУ
-- ============================================================

-- 2.1. Заказы на доставку
CREATE TABLE delivery_order (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Связь с заказом покупателя (если есть)
    sales_order_id BIGINT REFERENCES sales_order(id),
    -- Связь с заказом маркетплейса (если есть)
    marketplace_order_id BIGINT REFERENCES marketplace_order(id),

    -- Тип доставки
    delivery_type_code VARCHAR(20) NOT NULL REFERENCES delivery_type(code),

    -- Адрес доставки
    delivery_address TEXT NOT NULL,
    delivery_apartment VARCHAR(20),
    delivery_floor VARCHAR(10),
    delivery_entrance VARCHAR(10),
    delivery_intercom VARCHAR(20),
    delivery_comment TEXT,

    -- Контактные данные получателя
    recipient_name VARCHAR(255) NOT NULL,
    recipient_phone VARCHAR(20) NOT NULL,
    recipient_email VARCHAR(100),

    -- Геоданные (широта/долгота)
    latitude DECIMAL(10,7),
    longitude DECIMAL(10,7),

    -- Зона доставки
    delivery_zone_id BIGINT REFERENCES delivery_zone(id),

    -- Желаемое время доставки
    desired_delivery_date DATE,
    desired_time_from TIME,
    desired_time_to TIME,

    -- Фактическое время доставки
    actual_delivery_date DATE,
    actual_time_from TIME,
    actual_time_to TIME,

    -- Стоимость доставки
    delivery_price DECIMAL(15,2) NOT NULL DEFAULT 0,
    delivery_price_currency VARCHAR(3) DEFAULT 'RUB',

    -- Вес и габариты (для расчета стоимости)
    total_weight DECIMAL(10,3),
    total_volume DECIMAL(10,3),

    -- Курьерская служба
    carrier_id BIGINT REFERENCES delivery_carrier(id),

    -- Трек-номер
    tracking_number VARCHAR(100),
    tracking_url VARCHAR(255),

    -- Статус
    status_code VARCHAR(20) NOT NULL DEFAULT 'NEW' REFERENCES delivery_order_status(code),

    -- Кто принял заказ (сотрудник)
    created_by_id BIGINT REFERENCES employee(id),

    -- Дата и время создания
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    -- Дата и время завершения (доставлен/отменен)
    completed_at TIMESTAMP
);

COMMENT ON TABLE delivery_order IS 'Заказы на доставку';

CREATE INDEX idx_delivery_order_org ON delivery_order(organization_id);
CREATE INDEX idx_delivery_order_sales ON delivery_order(sales_order_id);
CREATE INDEX idx_delivery_order_marketplace ON delivery_order(marketplace_order_id);
CREATE INDEX idx_delivery_order_status ON delivery_order(status_code);
CREATE INDEX idx_delivery_order_tracking ON delivery_order(tracking_number);
CREATE INDEX idx_delivery_order_recipient ON delivery_order(recipient_phone);

-- 2.2. История изменения статусов доставки
CREATE TABLE delivery_order_status_history (
    id BIGSERIAL PRIMARY KEY,
    delivery_order_id BIGINT NOT NULL REFERENCES delivery_order(id) ON DELETE CASCADE,

    status_code VARCHAR(20) NOT NULL REFERENCES delivery_order_status(code),
    changed_by_id BIGINT REFERENCES employee(id),
    changed_at TIMESTAMP DEFAULT NOW(),

    -- Дополнительные данные (например, причина изменения)
    comment TEXT,

    -- Данные о местоположении (если есть)
    latitude DECIMAL(10,7),
    longitude DECIMAL(10,7),

    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE delivery_order_status_history IS 'История статусов доставки';

CREATE INDEX idx_delivery_status_history_order ON delivery_order_status_history(delivery_order_id);
CREATE INDEX idx_delivery_status_history_status ON delivery_order_status_history(status_code);

-- ============================================================
-- 3. КУРЬЕРЫ
-- ============================================================

-- 3.1. Курьеры (сотрудники с ролью курьера)
CREATE TABLE delivery_courier (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    employee_id BIGINT REFERENCES employee(id), -- если курьер является сотрудником

    -- Личные данные
    last_name VARCHAR(50) NOT NULL,
    first_name VARCHAR(50) NOT NULL,
    middle_name VARCHAR(50),
    phone VARCHAR(20) NOT NULL,
    email VARCHAR(100),

    -- Транспорт
    vehicle_type VARCHAR(50),
    vehicle_number VARCHAR(20),
    vehicle_capacity_kg DECIMAL(10,2),

    -- График работы
    work_schedule JSONB, -- массив дней недели с временем

    -- Зоны закрепления (массив ID зон)
    assigned_zone_ids BIGINT[],

    -- Статус
    is_active BOOLEAN DEFAULT TRUE,
    is_available BOOLEAN DEFAULT TRUE,

    -- Рейтинг (отзывы)
    rating DECIMAL(3,2) DEFAULT 5.0,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE delivery_courier IS 'Курьеры для доставки';

CREATE INDEX idx_courier_org ON delivery_courier(organization_id);
CREATE INDEX idx_courier_employee ON delivery_courier(employee_id);
CREATE INDEX idx_courier_phone ON delivery_courier(phone);
CREATE INDEX idx_courier_active ON delivery_courier(is_active) WHERE is_active = TRUE;

-- 3.2. Назначение курьера на заказ доставки
CREATE TABLE delivery_order_assignment (
    id BIGSERIAL PRIMARY KEY,
    delivery_order_id BIGINT NOT NULL REFERENCES delivery_order(id) ON DELETE CASCADE,
    courier_id BIGINT NOT NULL REFERENCES delivery_courier(id),

    -- Назначение
    assigned_at TIMESTAMP DEFAULT NOW(),
    assigned_by_id BIGINT REFERENCES employee(id),

    -- Принял ли курьер
    accepted_at TIMESTAMP,
    accepted_by_id BIGINT,

    -- Статус назначения
    status VARCHAR(20) DEFAULT 'ASSIGNED' CHECK (status IN ('ASSIGNED', 'ACCEPTED', 'REJECTED', 'COMPLETED', 'CANCELED')),

    -- Фактическое время начала доставки
    pickup_at TIMESTAMP,
    delivered_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE delivery_order_assignment IS 'Назначение курьера на заказ доставки';

CREATE INDEX idx_assignment_order ON delivery_order_assignment(delivery_order_id);
CREATE INDEX idx_assignment_courier ON delivery_order_assignment(courier_id);
CREATE INDEX idx_assignment_status ON delivery_order_assignment(status);

-- 3.3. График работы курьеров (закрепление на даты)
CREATE TABLE delivery_courier_schedule (
    id BIGSERIAL PRIMARY KEY,
    courier_id BIGINT NOT NULL REFERENCES delivery_courier(id) ON DELETE CASCADE,

    work_date DATE NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    breaks JSONB,

    is_working BOOLEAN DEFAULT TRUE,
    comment TEXT,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (courier_id, work_date)
);

COMMENT ON TABLE delivery_courier_schedule IS 'Индивидуальный график курьера на конкретные даты';

-- ============================================================
-- 4. ИНДЕКСЫ
-- ============================================================

CREATE INDEX idx_delivery_zone_org ON delivery_zone(organization_id);
CREATE INDEX idx_delivery_carrier_active ON delivery_carrier(is_active) WHERE is_active = TRUE;

CREATE INDEX idx_courier_schedule_courier ON delivery_courier_schedule(courier_id);
CREATE INDEX idx_courier_schedule_date ON delivery_courier_schedule(work_date);

-- ============================================================
-- 5. ТРИГГЕРЫ ДЛЯ ОБНОВЛЕНИЯ UPDATED_AT
-- ============================================================

DO $$
DECLARE
    table_name text;
BEGIN
    FOR table_name IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename IN ('delivery_carrier', 'delivery_zone', 'delivery_order', 'delivery_courier', 'delivery_order_assignment', 'delivery_courier_schedule')
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
-- 6. МАТЕРИАЛИЗОВАННОЕ ПРЕДСТАВЛЕНИЕ ДЛЯ СТАТИСТИКИ ДОСТАВКИ
-- ============================================================

CREATE MATERIALIZED VIEW mv_delivery_stats AS
SELECT
    o.id AS organization_id,
    o.short_name AS organization_name,
    COUNT(d.id) AS total_orders,
    COUNT(d.id) FILTER (WHERE d.status_code = 'DELIVERED') AS delivered,
    COUNT(d.id) FILTER (WHERE d.status_code = 'CANCELED') AS canceled,
    AVG(EXTRACT(EPOCH FROM (d.completed_at - d.created_at)) / 3600) AS avg_delivery_hours,
    SUM(d.delivery_price) AS total_delivery_revenue,
    AVG(d.delivery_price) AS avg_delivery_price,
    COUNT(DISTINCT d.carrier_id) AS carriers_used
FROM delivery_order d
JOIN organization o ON d.organization_id = o.id
GROUP BY o.id, o.short_name
WITH NO DATA;

CREATE UNIQUE INDEX idx_mv_delivery_stats_org ON mv_delivery_stats(organization_id);

COMMENT ON MATERIALIZED VIEW mv_delivery_stats IS 'Статистика по доставкам';

-- ============================================================
-- 7. ФУНКЦИИ ДЛЯ РАБОТЫ С ДОСТАВКОЙ
-- ============================================================

-- 7.1. Создание заказа на доставку из заказа покупателя
CREATE OR REPLACE FUNCTION create_delivery_from_sales_order(
    p_sales_order_id BIGINT,
    p_delivery_type_code VARCHAR,
    p_carrier_id BIGINT DEFAULT NULL,
    p_delivery_zone_id BIGINT DEFAULT NULL,
    p_delivery_price DECIMAL DEFAULT NULL
) RETURNS BIGINT AS $$
DECLARE
    v_order record;
    v_delivery_id BIGINT;
    v_zone_price DECIMAL;
BEGIN
    -- Получаем данные заказа
    SELECT so.*, c.full_name AS customer_name, c.phone AS customer_phone, c.email AS customer_email
    INTO v_order
    FROM sales_order so
    JOIN counterparty c ON so.buyer_id = c.id
    WHERE so.id = p_sales_order_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'Заказ с ID % не найден', p_sales_order_id;
    END IF;

    -- Если цена не указана, пытаемся получить из зоны
    IF p_delivery_price IS NULL AND p_delivery_zone_id IS NOT NULL THEN
        SELECT base_price INTO v_zone_price
        FROM delivery_zone
        WHERE id = p_delivery_zone_id;
        p_delivery_price := COALESCE(v_zone_price, 0);
    END IF;

    -- Создаем доставку
    INSERT INTO delivery_order (
        organization_id,
        sales_order_id,
        delivery_type_code,
        delivery_address,
        recipient_name,
        recipient_phone,
        recipient_email,
        delivery_zone_id,
        delivery_price,
        carrier_id,
        status_code,
        created_by_id
    ) VALUES (
        v_order.organization_id,
        p_sales_order_id,
        p_delivery_type_code,
        COALESCE(v_order.delivery_address, 'Не указан'),
        v_order.customer_name,
        v_order.customer_phone,
        v_order.customer_email,
        p_delivery_zone_id,
        COALESCE(p_delivery_price, 0),
        p_carrier_id,
        'NEW',
        v_order.manager_id
    ) RETURNING id INTO v_delivery_id;

    -- Обновляем заказ, связывая с доставкой (если нужно добавить поле delivery_order_id в sales_order, но мы не добавляем)

    RETURN v_delivery_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION create_delivery_from_sales_order IS 'Создание доставки на основе заказа покупателя';

-- 7.2. Назначение курьера на доставку
CREATE OR REPLACE FUNCTION assign_courier_to_delivery(
    p_delivery_order_id BIGINT,
    p_courier_id BIGINT,
    p_assigned_by_id BIGINT
) RETURNS BOOLEAN AS $$
DECLARE
    v_courier_available BOOLEAN;
BEGIN
    -- Проверяем доступность курьера
    SELECT is_available INTO v_courier_available
    FROM delivery_courier
    WHERE id = p_courier_id AND is_active = TRUE;

    IF NOT v_courier_available THEN
        RAISE EXCEPTION 'Курьер недоступен или неактивен';
    END IF;

    -- Назначаем
    INSERT INTO delivery_order_assignment (
        delivery_order_id,
        courier_id,
        assigned_by_id,
        status
    ) VALUES (
        p_delivery_order_id,
        p_courier_id,
        p_assigned_by_id,
        'ASSIGNED'
    );

    -- Обновляем статус доставки
    UPDATE delivery_order
    SET status_code = 'ASSIGNED',
        updated_at = NOW()
    WHERE id = p_delivery_order_id;

    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION assign_courier_to_delivery IS 'Назначение курьера на доставку';

-- 7.3. Изменение статуса доставки с записью истории
CREATE OR REPLACE FUNCTION update_delivery_status(
    p_delivery_order_id BIGINT,
    p_new_status VARCHAR,
    p_changed_by_id BIGINT,
    p_comment TEXT DEFAULT NULL,
    p_latitude DECIMAL DEFAULT NULL,
    p_longitude DECIMAL DEFAULT NULL
) RETURNS VOID AS $$
BEGIN
    -- Обновляем статус в основном заказе
    UPDATE delivery_order
    SET
        status_code = p_new_status,
        updated_at = NOW(),
        completed_at = CASE WHEN p_new_status IN ('DELIVERED', 'CANCELED', 'RETURNED') THEN NOW() ELSE completed_at END
    WHERE id = p_delivery_order_id;

    -- Записываем историю
    INSERT INTO delivery_order_status_history (
        delivery_order_id,
        status_code,
        changed_by_id,
        comment,
        latitude,
        longitude
    ) VALUES (
        p_delivery_order_id,
        p_new_status,
        p_changed_by_id,
        p_comment,
        p_latitude,
        p_longitude
    );

    -- Если статус доставлен, обновляем связанные заказы (например, маркетплейс)
    IF p_new_status = 'DELIVERED' THEN
        -- Можно добавить логику обновления статуса в marketplace_order
        -- Например, UPDATE marketplace_order SET marketplace_status = 'DELIVERED' WHERE delivery_order_id = ...
        NULL;
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_delivery_status IS 'Обновление статуса доставки с историей';

-- ============================================================
-- 8. ПРИМЕРЫ ИСПОЛЬЗОВАНИЯ (закомментированы)
-- ============================================================

-- -- Создание доставки для заказа
-- SELECT create_delivery_from_sales_order(
--     1, -- sales_order_id
--     'COURIER', -- delivery_type_code
--     NULL, -- carrier_id
--     (SELECT id FROM delivery_zone WHERE name = 'Центр' LIMIT 1), -- zone
--     500 -- price
-- );

-- -- Назначение курьера
-- SELECT assign_courier_to_delivery(1, 1, 1);

-- -- Обновление статуса
-- CALL update_delivery_status(1, 'PICKED_UP', 1, 'Курьер забрал заказ');
