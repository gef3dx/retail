-- Этап 4: касса 54-ФЗ + ОФД (по sql/01 cash_* + sales_receipt* и sql/02 ofd_*).
-- Отложено: warehouse_id/employee FK (этапы 6-7, пока ссылаемся на users),
-- sales_order_id (этап 5), receipt_marking_link (этап 5), ЕГАИС/алкоголь (этап 9),
-- детальные таблицы ФФД из 03 (атрибуты храним кодами в позициях).

-- Справочники ФФД (коды из 03, только нужное кассе).
CREATE TABLE IF NOT EXISTS ffd_item_attribute (
    code VARCHAR(30) PRIMARY KEY,
    name VARCHAR(100) NOT NULL
);
INSERT INTO ffd_item_attribute (code, name) VALUES
('GOOD', 'Товар'), ('SERVICE', 'Услуга'), ('WORK', 'Работа'),
('EXCISABLE', 'Подакцизный товар'), ('MARKED', 'Маркированный товар')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS ffd_payment_method (
    code VARCHAR(30) PRIMARY KEY,
    name VARCHAR(100) NOT NULL
);
INSERT INTO ffd_payment_method (code, name) VALUES
('FULL', 'Полный расчет'), ('PREPAY', 'Предоплата'),
('ADVANCE', 'Аванс'), ('CREDIT', 'Кредит/рассрочка')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS cash_register (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    reg_number VARCHAR(20) UNIQUE NOT NULL,
    model VARCHAR(50) NOT NULL DEFAULT 'MOCK-KKT',
    factory_number VARCHAR(30) NOT NULL DEFAULT 'MOCK',
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','BLOCKED')),
    installation_address TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cash_shift (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    cash_register_id BIGINT NOT NULL REFERENCES cash_register(id) ON DELETE CASCADE,
    shift_number INT NOT NULL,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    opened_by_id BIGINT REFERENCES users(id),
    closed_at TIMESTAMPTZ,
    closed_by_id BIGINT REFERENCES users(id),
    start_cash DECIMAL(15,2) DEFAULT 0,
    actual_cash DECIMAL(15,2),
    status VARCHAR(20) DEFAULT 'OPEN' CHECK (status IN ('OPEN','CLOSED')),
    x_report JSONB,
    z_report JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (cash_register_id, shift_number)
);
-- Только одна открытая смена на кассу.
CREATE UNIQUE INDEX IF NOT EXISTS idx_shift_open_once
    ON cash_shift(cash_register_id) WHERE status = 'OPEN';

CREATE TABLE IF NOT EXISTS sales_receipt (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    shift_id BIGINT NOT NULL REFERENCES cash_shift(id) ON DELETE RESTRICT,
    cash_register_id BIGINT NOT NULL REFERENCES cash_register(id),
    cashier_id BIGINT REFERENCES users(id),
    receipt_number VARCHAR(20) NOT NULL,
    receipt_type VARCHAR(20) DEFAULT 'SALE' CHECK (receipt_type IN ('SALE','RETURN','CORRECTION')),
    -- Для RETURN: исходный чек; для CORRECTION: основание текстом.
    base_receipt_id BIGINT REFERENCES sales_receipt(id),
    correction_reason TEXT,
    total_amount DECIMAL(15,2) NOT NULL CHECK (total_amount >= 0),
    total_vat DECIMAL(15,2) DEFAULT 0,
    payment_type VARCHAR(20) NOT NULL CHECK (payment_type IN ('CASH','CARD','MIXED')),
    payment_cash DECIMAL(15,2) DEFAULT 0,
    payment_card DECIMAL(15,2) DEFAULT 0,
    change_amount DECIMAL(15,2) DEFAULT 0,
    has_marked_goods BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (cash_register_id, receipt_number)
);

CREATE TABLE IF NOT EXISTS sales_receipt_item (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES sales_receipt(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    product_name VARCHAR(255) NOT NULL,
    product_sku VARCHAR(50),
    quantity DECIMAL(15,3) NOT NULL CHECK (quantity > 0),
    price DECIMAL(15,2) NOT NULL CHECK (price >= 0),
    vat_rate DECIMAL(5,2) NOT NULL DEFAULT 20.00,
    vat_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    discount DECIMAL(15,2) DEFAULT 0,
    is_marked BOOLEAN DEFAULT FALSE,
    ffd_item_attribute VARCHAR(30) DEFAULT 'GOOD' REFERENCES ffd_item_attribute(code),
    ffd_payment_method VARCHAR(30) DEFAULT 'FULL' REFERENCES ffd_payment_method(code)
);
CREATE INDEX IF NOT EXISTS idx_receipt_item_receipt ON sales_receipt_item(receipt_id);

-- Настройки ОФД (провайдер mock по умолчанию; настоящий — этап 9).
CREATE TABLE IF NOT EXISTS ofd_settings (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    provider VARCHAR(30) NOT NULL DEFAULT 'MOCK',
    api_url VARCHAR(255),
    auto_send_enabled BOOLEAN DEFAULT TRUE,
    max_retries INT DEFAULT 3,
    -- Тестовый крючок: сколько первых попыток по чеку проваливать (0 = всегда успех).
    fail_first_attempts INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, provider)
);

-- Очередь отправки чеков в ОФД (02: ofd_send_status, один-к-одному с чеком).
CREATE TABLE IF NOT EXISTS ofd_send_status (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL UNIQUE REFERENCES sales_receipt(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    send_attempt INT DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    status VARCHAR(20) DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','SENT','COMPLETED','FAILED','RETRY')),
    fiscal_document_number VARCHAR(20),
    fiscal_sign VARCHAR(30),
    qr_code_url VARCHAR(255),
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ofd_pending ON ofd_send_status(status) WHERE status IN ('PENDING','RETRY');
