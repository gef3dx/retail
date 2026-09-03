-- ============================================================
-- 05_services_booking.sql – Услуги и бронирование
-- ============================================================

-- ============================================================
-- 1. ДОБАВЛЯЕМ ПОЛЯ В ТАБЛИЦУ ТОВАРОВ
-- ============================================================

-- Добавляем тип продукта (товар или услуга)
ALTER TABLE catalog_product ADD COLUMN product_type VARCHAR(20) DEFAULT 'GOODS' CHECK (product_type IN ('GOODS', 'SERVICE'));
COMMENT ON COLUMN catalog_product.product_type IS 'Тип продукта: GOODS - товар, SERVICE - услуга';

-- Добавляем длительность услуги (в минутах)
ALTER TABLE catalog_product ADD COLUMN service_duration_minutes INT;
COMMENT ON COLUMN catalog_product.service_duration_minutes IS 'Длительность услуги в минутах (только для услуг)';

-- Добавляем флаг, требуется ли предварительная запись для услуги
ALTER TABLE catalog_product ADD COLUMN service_requires_booking BOOLEAN DEFAULT FALSE;
COMMENT ON COLUMN catalog_product.service_requires_booking IS 'Требуется ли предварительная запись для услуги';

-- Добавляем флаг, можно ли продать услугу без бронирования (например, экспресс-услуга)
ALTER TABLE catalog_product ADD COLUMN service_allow_walk_in BOOLEAN DEFAULT TRUE;
COMMENT ON COLUMN catalog_product.service_allow_walk_in IS 'Можно ли оказать услугу без предварительной записи (по факту прихода)';

-- Добавляем флаг, активна ли услуга для бронирования
ALTER TABLE catalog_product ADD COLUMN service_booking_enabled BOOLEAN DEFAULT TRUE;
COMMENT ON COLUMN catalog_product.service_booking_enabled IS 'Включено ли бронирование для данной услуги';

-- ============================================================
-- 2. СПРАВОЧНИКИ ДЛЯ БРОНИРОВАНИЯ
-- ============================================================

-- 2.1. Типы ресурсов (помещение, оборудование, сотрудник)
CREATE TABLE service_resource_type (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE
);

COMMENT ON TABLE service_resource_type IS 'Типы ресурсов для бронирования услуг';

INSERT INTO service_resource_type (code, name, description) VALUES
('ROOM', 'Помещение', 'Кабинет, зал, комната'),
('EQUIPMENT', 'Оборудование', 'Специализированное оборудование'),
('EMPLOYEE', 'Сотрудник', 'Конкретный сотрудник, оказывающий услугу');

-- 2.2. Ресурсы (конкретные экземпляры)
CREATE TABLE service_resource (
    id BIGSERIAL PRIMARY KEY,
    resource_type_code VARCHAR(20) NOT NULL REFERENCES service_resource_type(code),
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    name VARCHAR(100) NOT NULL,
    description TEXT,
    code VARCHAR(50) UNIQUE,

    -- Для сотрудников – ссылка на employee
    employee_id BIGINT REFERENCES employee(id),

    -- Для помещений – адрес или номер
    location VARCHAR(255),

    -- Признаки
    is_active BOOLEAN DEFAULT TRUE,

    -- Настройки времени (перерыв, выходные) – можно хранить JSON
    schedule_config JSONB DEFAULT '{}'::JSONB,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE service_resource IS 'Ресурсы, используемые при оказании услуг (кабинеты, оборудование, сотрудники)';

CREATE INDEX idx_resource_org ON service_resource(organization_id);
CREATE INDEX idx_resource_type ON service_resource(resource_type_code);
CREATE INDEX idx_resource_employee ON service_resource(employee_id);

-- 2.3. Связь услуги с необходимыми ресурсами (какие ресурсы требуются для услуги)
CREATE TABLE service_product_resource (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,

    -- Количество ресурса, необходимое для услуги (например, 1 кабинет, 1 сотрудник)
    quantity INT NOT NULL DEFAULT 1,

    -- Обязательность (если ресурс обязателен, то без него услуга не может быть оказана)
    is_mandatory BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (product_id, resource_id)
);

COMMENT ON TABLE service_product_resource IS 'Какие ресурсы требуются для оказания услуги';

CREATE INDEX idx_service_product_resource_product ON service_product_resource(product_id);
CREATE INDEX idx_service_product_resource_resource ON service_product_resource(resource_id);

-- ============================================================
-- 3. БРОНИРОВАНИЯ
-- ============================================================

-- 3.1. Статусы бронирования (справочник)
CREATE TABLE service_booking_status (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_terminal BOOLEAN DEFAULT FALSE
);

INSERT INTO service_booking_status (code, name, description, is_terminal) VALUES
('PENDING', 'Ожидает подтверждения', 'Бронирование создано, ожидает подтверждения', FALSE),
('CONFIRMED', 'Подтверждено', 'Бронирование подтверждено', FALSE),
('IN_PROGRESS', 'Выполняется', 'Услуга оказывается', FALSE),
('COMPLETED', 'Выполнено', 'Услуга оказана', TRUE),
('CANCELED', 'Отменено', 'Бронирование отменено', TRUE),
('NO_SHOW', 'Клиент не явился', 'Клиент не пришел', TRUE);

-- 3.2. Бронирования услуг
CREATE TABLE service_booking (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Клиент (может быть физ. лицо или юр. лицо)
    customer_id BIGINT REFERENCES counterparty(id),
    -- Если клиент не зарегистрирован в системе, можно хранить контактные данные
    customer_name VARCHAR(255),
    customer_phone VARCHAR(20),
    customer_email VARCHAR(100),

    -- Дата и время начала и окончания
    start_datetime TIMESTAMP NOT NULL,
    end_datetime TIMESTAMP,

    -- Длительность (может переопределять длительность услуги)
    duration_minutes INT NOT NULL,

    -- Ответственный сотрудник (кто оказывает) – можно указать основного
    employee_id BIGINT REFERENCES employee(id),

    -- Статус
    status_code VARCHAR(20) NOT NULL DEFAULT 'PENDING' REFERENCES service_booking_status(code),

    -- Комментарий
    notes TEXT,
    internal_notes TEXT,

    -- Связь с продажей (если услуга уже оплачена через чек)
    sales_receipt_item_id BIGINT REFERENCES sales_receipt_item(id),
    sales_order_line_id BIGINT REFERENCES sales_order_line(id),

    -- Связь с заказом (если бронирование связано с заказом)
    sales_order_id BIGINT REFERENCES sales_order(id),

    -- Кто создал и изменил
    created_by_id BIGINT REFERENCES users(id),
    updated_by_id BIGINT REFERENCES users(id),

    -- Временные метки
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    confirmed_at TIMESTAMP,
    completed_at TIMESTAMP,
    canceled_at TIMESTAMP,

    -- Флаг уведомления
    notification_sent BOOLEAN DEFAULT FALSE
);

COMMENT ON TABLE service_booking IS 'Бронирования услуг';

CREATE INDEX idx_booking_org ON service_booking(organization_id);
CREATE INDEX idx_booking_customer ON service_booking(customer_id);
CREATE INDEX idx_booking_status ON service_booking(status_code);
CREATE INDEX idx_booking_start ON service_booking(start_datetime);
CREATE INDEX idx_booking_employee ON service_booking(employee_id);
CREATE INDEX idx_booking_sales_receipt ON service_booking(sales_receipt_item_id);
CREATE INDEX idx_booking_sales_order ON service_booking(sales_order_id);

-- 3.3. Позиции бронирования (если в бронировании несколько услуг)
CREATE TABLE service_booking_item (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES service_booking(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),

    -- Количество (обычно 1, но может быть несколько)
    quantity INT NOT NULL DEFAULT 1,

    -- Цена на момент бронирования (может отличаться от текущей)
    price DECIMAL(15,2) NOT NULL,
    total_amount DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity) STORED,

    -- Длительность для этой позиции (если отличается от общей)
    duration_minutes INT,

    -- Связь с продажей
    sales_receipt_item_id BIGINT REFERENCES sales_receipt_item(id),
    sales_order_line_id BIGINT REFERENCES sales_order_line(id),

    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE service_booking_item IS 'Позиции (услуги) в бронировании';

CREATE INDEX idx_booking_item_booking ON service_booking_item(booking_id);
CREATE INDEX idx_booking_item_product ON service_booking_item(product_id);

-- 3.4. Ресурсы, закрепленные за бронированием (на конкретные дату/время)
CREATE TABLE service_booking_resource (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES service_booking(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,

    -- Если ресурс закреплён за конкретной позицией услуги
    booking_item_id BIGINT REFERENCES service_booking_item(id) ON DELETE CASCADE,

    -- Статус подтверждения ресурса (например, подтвержден ли сотрудник)
    is_confirmed BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (booking_id, resource_id, booking_item_id)
);

COMMENT ON TABLE service_booking_resource IS 'Ресурсы, закрепленные за бронированием (сотрудник, кабинет, оборудование)';

CREATE INDEX idx_booking_resource_booking ON service_booking_resource(booking_id);
CREATE INDEX idx_booking_resource_resource ON service_booking_resource(resource_id);

-- 3.5. История изменений статусов бронирования (для аудита)
CREATE TABLE service_booking_status_history (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES service_booking(id) ON DELETE CASCADE,
    status_code VARCHAR(20) NOT NULL REFERENCES service_booking_status(code),
    changed_by_id BIGINT REFERENCES users(id),
    changed_at TIMESTAMP DEFAULT NOW(),
    comment TEXT
);

COMMENT ON TABLE service_booking_status_history IS 'История изменения статусов бронирования';

CREATE INDEX idx_booking_status_history_booking ON service_booking_status_history(booking_id);

-- ============================================================
-- 4. ГРАФИК РАБОТЫ РЕСУРСОВ (расписание)
-- ============================================================

-- 4.1. Рабочие часы ресурсов (ежедневное расписание)
CREATE TABLE service_resource_schedule (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,

    -- День недели (1-7, где 1 - Понедельник)
    day_of_week INT NOT NULL CHECK (day_of_week BETWEEN 1 AND 7),

    -- Время начала и окончания работы
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,

    -- Перерывы (можно хранить JSON)
    breaks JSONB DEFAULT '[]'::JSONB,

    -- Активность
    is_active BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (resource_id, day_of_week)
);

COMMENT ON TABLE service_resource_schedule IS 'Расписание работы ресурсов по дням недели';

-- 4.2. Исключения в расписании (праздники, отгулы)
CREATE TABLE service_resource_schedule_exception (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES service_resource(id) ON DELETE CASCADE,

    exception_date DATE NOT NULL,
    is_working BOOLEAN DEFAULT FALSE,

    start_time TIME,
    end_time TIME,
    breaks JSONB DEFAULT '[]'::JSONB,

    reason TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (resource_id, exception_date)
);

COMMENT ON TABLE service_resource_schedule_exception IS 'Исключения в расписании ресурсов (праздники, отгулы)';

-- ============================================================
-- 5. ИНДЕКСЫ ДЛЯ НОВЫХ ТАБЛИЦ
-- ============================================================

CREATE INDEX idx_catalog_product_type ON catalog_product(product_type) WHERE product_type = 'SERVICE';
CREATE INDEX idx_catalog_product_service_booking ON catalog_product(service_booking_enabled) WHERE service_booking_enabled = TRUE;

CREATE INDEX idx_resource_schedule_resource ON service_resource_schedule(resource_id);
CREATE INDEX idx_resource_schedule_day ON service_resource_schedule(day_of_week);
CREATE INDEX idx_resource_schedule_exception_resource ON service_resource_schedule_exception(resource_id);
CREATE INDEX idx_resource_schedule_exception_date ON service_resource_schedule_exception(exception_date);

-- ============================================================
-- 6. ТРИГГЕРЫ ДЛЯ АВТОМАТИЧЕСКОГО РАСЧЕТА END_DATETIME
-- ============================================================

-- Функция для автоматического вычисления end_datetime по start_datetime + duration_minutes
CREATE OR REPLACE FUNCTION calculate_booking_end_datetime()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.start_datetime IS NOT NULL AND NEW.duration_minutes IS NOT NULL THEN
        NEW.end_datetime := NEW.start_datetime + (NEW.duration_minutes || ' minutes')::INTERVAL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION calculate_booking_end_datetime IS 'Вычисляет окончание бронирования по началу и длительности';

CREATE TRIGGER trg_service_booking_calc_end
BEFORE INSERT OR UPDATE OF start_datetime, duration_minutes ON service_booking
FOR EACH ROW
EXECUTE FUNCTION calculate_booking_end_datetime();

-- ============================================================
-- 7. ФУНКЦИИ ДЛЯ РАБОТЫ С БРОНИРОВАНИЯМИ
-- ============================================================

-- 7.1. Проверка доступности ресурсов на заданный временной интервал
CREATE OR REPLACE FUNCTION check_resource_availability(
    p_resource_id BIGINT,
    p_start_datetime TIMESTAMP,
    p_duration_minutes INT
) RETURNS BOOLEAN AS $$
DECLARE
    v_end_datetime TIMESTAMP := p_start_datetime + (p_duration_minutes || ' minutes')::INTERVAL;
    v_conflict_count INT;
BEGIN
    -- Проверяем пересечения с существующими бронированиями, у которых статус не финальный
    SELECT COUNT(*)
    INTO v_conflict_count
    FROM service_booking b
    JOIN service_booking_resource br ON b.id = br.booking_id
    WHERE br.resource_id = p_resource_id
    AND b.status_code NOT IN ('COMPLETED', 'CANCELED', 'NO_SHOW')
    AND (
        (b.start_datetime < v_end_datetime AND b.end_datetime > p_start_datetime)
        OR (b.start_datetime >= p_start_datetime AND b.start_datetime < v_end_datetime)
    );

    RETURN v_conflict_count = 0;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION check_resource_availability IS 'Проверяет, свободен ли ресурс на указанный интервал времени';

-- 7.2. Создание бронирования (с проверкой доступности)
CREATE OR REPLACE FUNCTION create_service_booking(
    p_organization_id BIGINT,
    p_customer_id BIGINT,
    p_customer_name VARCHAR,
    p_customer_phone VARCHAR,
    p_customer_email VARCHAR,
    p_start_datetime TIMESTAMP,
    p_duration_minutes INT,
    p_employee_id BIGINT,
    p_product_id BIGINT,
    p_price DECIMAL,
    p_notes TEXT,
    p_created_by_id BIGINT
) RETURNS BIGINT AS $$
DECLARE
    v_booking_id BIGINT;
    v_resource_id BIGINT;
    v_is_available BOOLEAN;
BEGIN
    -- Получаем ресурс, связанный с услугой (если есть обязательный ресурс)
    -- Здесь предполагаем, что услуга может требовать ресурс типа EMPLOYEE (сотрудник)
    -- Если передан employee_id, используем его как ресурс
    IF p_employee_id IS NOT NULL THEN
        -- Находим resource_id для этого сотрудника
        SELECT id INTO v_resource_id
        FROM service_resource
        WHERE employee_id = p_employee_id AND resource_type_code = 'EMPLOYEE' AND is_active = TRUE
        LIMIT 1;

        IF v_resource_id IS NULL THEN
            -- Если нет ресурса для сотрудника, создаем его автоматически (для упрощения)
            INSERT INTO service_resource (resource_type_code, organization_id, name, employee_id, is_active)
            VALUES ('EMPLOYEE', p_organization_id, (SELECT last_name || ' ' || first_name FROM employee WHERE id = p_employee_id), p_employee_id, TRUE)
            RETURNING id INTO v_resource_id;
        END IF;

        -- Проверяем доступность
        v_is_available := check_resource_availability(v_resource_id, p_start_datetime, p_duration_minutes);
        IF NOT v_is_available THEN
            RAISE EXCEPTION 'Сотрудник уже занят в указанное время';
        END IF;
    END IF;

    -- Создаем бронирование
    INSERT INTO service_booking (
        organization_id,
        customer_id,
        customer_name,
        customer_phone,
        customer_email,
        start_datetime,
        duration_minutes,
        employee_id,
        status_code,
        notes,
        created_by_id
    ) VALUES (
        p_organization_id,
        p_customer_id,
        p_customer_name,
        p_customer_phone,
        p_customer_email,
        p_start_datetime,
        p_duration_minutes,
        p_employee_id,
        'PENDING',
        p_notes,
        p_created_by_id
    ) RETURNING id INTO v_booking_id;

    -- Добавляем позицию услуги
    INSERT INTO service_booking_item (
        booking_id,
        product_id,
        quantity,
        price,
        duration_minutes
    ) VALUES (
        v_booking_id,
        p_product_id,
        1,
        p_price,
        p_duration_minutes
    );

    -- Если есть ресурс, привязываем его к бронированию
    IF v_resource_id IS NOT NULL THEN
        INSERT INTO service_booking_resource (booking_id, resource_id, is_confirmed)
        VALUES (v_booking_id, v_resource_id, TRUE);
    END IF;

    -- Записываем историю статуса
    INSERT INTO service_booking_status_history (booking_id, status_code, changed_by_id, comment)
    VALUES (v_booking_id, 'PENDING', p_created_by_id, 'Создано бронирование');

    RETURN v_booking_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION create_service_booking IS 'Создание нового бронирования с проверкой доступности ресурсов';

-- 7.3. Изменение статуса бронирования с записью истории
CREATE OR REPLACE FUNCTION update_booking_status(
    p_booking_id BIGINT,
    p_new_status VARCHAR,
    p_changed_by_id BIGINT,
    p_comment TEXT DEFAULT NULL
) RETURNS VOID AS $$
BEGIN
    -- Обновляем статус
    UPDATE service_booking
    SET
        status_code = p_new_status,
        updated_at = NOW(),
        confirmed_at = CASE WHEN p_new_status = 'CONFIRMED' THEN NOW() ELSE confirmed_at END,
        completed_at = CASE WHEN p_new_status = 'COMPLETED' THEN NOW() ELSE completed_at END,
        canceled_at = CASE WHEN p_new_status IN ('CANCELED', 'NO_SHOW') THEN NOW() ELSE canceled_at END,
        updated_by_id = p_changed_by_id
    WHERE id = p_booking_id;

    -- Записываем историю
    INSERT INTO service_booking_status_history (booking_id, status_code, changed_by_id, comment)
    VALUES (p_booking_id, p_new_status, p_changed_by_id, p_comment);
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_booking_status IS 'Обновление статуса бронирования с сохранением истории';

-- ============================================================
-- 8. МАТЕРИАЛИЗОВАННОЕ ПРЕДСТАВЛЕНИЕ ДЛЯ АНАЛИТИКИ УСЛУГ
-- ============================================================

-- 8.1. Сводка по услугам (количество бронирований, выручка)
CREATE MATERIALIZED VIEW mv_service_analytics AS
SELECT
    p.id AS product_id,
    p.sku,
    p.name,
    p.product_type,
    COUNT(b.id) AS total_bookings,
    COUNT(b.id) FILTER (WHERE b.status_code = 'COMPLETED') AS completed_bookings,
    COUNT(b.id) FILTER (WHERE b.status_code = 'CANCELED') AS canceled_bookings,
    COUNT(b.id) FILTER (WHERE b.status_code = 'NO_SHOW') AS no_show_bookings,
    COALESCE(SUM(bi.total_amount), 0) AS total_revenue,
    AVG(bi.total_amount) AS avg_revenue_per_booking,
    MIN(b.start_datetime) AS first_booking,
    MAX(b.start_datetime) AS last_booking
FROM catalog_product p
LEFT JOIN service_booking_item bi ON p.id = bi.product_id
LEFT JOIN service_booking b ON bi.booking_id = b.id
WHERE p.product_type = 'SERVICE'
GROUP BY p.id, p.sku, p.name, p.product_type
WITH NO DATA;

CREATE UNIQUE INDEX idx_mv_service_analytics_product ON mv_service_analytics(product_id);

COMMENT ON MATERIALIZED VIEW mv_service_analytics IS 'Аналитика по услугам';

-- ============================================================
-- 9. ПРИМЕРЫ ИСПОЛЬЗОВАНИЯ (закомментированы)
-- ============================================================

-- -- Создание услуги
-- INSERT INTO catalog_product (sku, name, product_type, service_duration_minutes, service_requires_booking, vat_rate, measure_unit)
-- VALUES ('SVC-001', 'Консультация юриста', 'SERVICE', 60, TRUE, 20, 'шт');

-- -- Создание ресурса (сотрудник)
-- INSERT INTO service_resource (resource_type_code, organization_id, name, employee_id)
-- VALUES ('EMPLOYEE', 1, 'Иванов Иван', (SELECT id FROM employee WHERE last_name = 'Иванов' LIMIT 1));

-- -- Связь услуги с ресурсом
-- INSERT INTO service_product_resource (product_id, resource_id)
-- VALUES ((SELECT id FROM catalog_product WHERE sku = 'SVC-001'), (SELECT id FROM service_resource WHERE name = 'Иванов Иван'));

-- -- Создание бронирования
-- SELECT create_service_booking(
--     1, -- organization_id
--     NULL, -- customer_id (аноним)
--     'Петров Петр', -- customer_name
--     '+79991234567', -- customer_phone
--     NULL, -- email
--     '2026-09-04 10:00:00'::TIMESTAMP, -- start
--     60, -- duration_minutes
--     (SELECT id FROM employee WHERE last_name = 'Иванов' LIMIT 1), -- employee_id
--     (SELECT id FROM catalog_product WHERE sku = 'SVC-001'), -- product_id
--     5000, -- price
--     'Первая консультация',
--     1 -- created_by_id
-- );

-- -- Подтверждение бронирования
-- SELECT update_booking_status(1, 'CONFIRMED', 1, 'Подтверждено менеджером');
