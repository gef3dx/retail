-- Этап 3: каталог минимум для кассы (по sql/01 core + sql/07 statuses).
-- Отложено на later: counterparty/employee/warehouse FK, партии, пул маркировки (этап 5-6).
-- Отличия от 01: category_id/brand_id BIGINT (в 01 ошибочно INT),
-- price_type.code уникален в пределах организации, а не глобально.

CREATE TABLE IF NOT EXISTS catalog_category (
    id BIGSERIAL PRIMARY KEY,
    parent_id BIGINT REFERENCES catalog_category(id) ON DELETE SET NULL,
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    is_marked_by_default BOOLEAN DEFAULT FALSE,
    sort_order INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS catalog_brand (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    country VARCHAR(50),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Справочник статусов (sql/07) — нужен до catalog_product из-за FK.
CREATE TABLE IF NOT EXISTS product_status (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_terminal BOOLEAN DEFAULT FALSE,
    sort_order INT DEFAULT 0
);
INSERT INTO product_status (code, name, is_terminal, sort_order) VALUES
('ACTIVE', 'Активный', FALSE, 10),
('INACTIVE', 'Неактивный', FALSE, 20),
('NEW', 'Новинка', FALSE, 5),
('PROMOTION', 'Распродажа', FALSE, 15),
('SOON', 'Скоро в продаже', FALSE, 0),
('DISCONTINUED', 'Снят с продажи', TRUE, 30),
('OUT_OF_STOCK', 'Нет в наличии', FALSE, 25),
('PREORDER', 'Предзаказ', FALSE, 3)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS catalog_product (
    id BIGSERIAL PRIMARY KEY,
    sku VARCHAR(50) UNIQUE NOT NULL,
    gtin VARCHAR(14) UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    category_id BIGINT REFERENCES catalog_category(id) ON DELETE SET NULL,
    brand_id BIGINT REFERENCES catalog_brand(id) ON DELETE SET NULL,
    measure_unit VARCHAR(10) DEFAULT 'шт',
    base_price DECIMAL(15,2),
    vat_rate DECIMAL(5,2) DEFAULT 20.00,
    is_marked BOOLEAN DEFAULT FALSE,
    marking_type VARCHAR(20),
    status_code VARCHAR(20) DEFAULT 'ACTIVE' REFERENCES product_status(code),
    status_start_date DATE,
    status_end_date DATE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_category ON catalog_product(category_id);
CREATE INDEX IF NOT EXISTS idx_product_brand ON catalog_product(brand_id);
CREATE INDEX IF NOT EXISTS idx_product_marked ON catalog_product(is_marked) WHERE is_marked = TRUE;
CREATE INDEX IF NOT EXISTS idx_product_status ON catalog_product(status_code);
CREATE INDEX IF NOT EXISTS idx_product_name ON catalog_product(name);
CREATE INDEX IF NOT EXISTS idx_product_gtin ON catalog_product(gtin) WHERE gtin IS NOT NULL;

-- Упаковки (нужны этапу 5 для КИГУ/КИТУ + поиск по штрихкоду упаковки).
CREATE TABLE IF NOT EXISTS product_packaging (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE CASCADE,
    packaging_type VARCHAR(20) NOT NULL CHECK (packaging_type IN ('UNIT', 'GROUP', 'TRANSPORT', 'OSU')),
    gtin_packaging VARCHAR(14) NOT NULL,
    units_in_pack INT NOT NULL DEFAULT 1,
    is_default BOOLEAN DEFAULT FALSE,
    UNIQUE (product_id, gtin_packaging)
);
CREATE INDEX IF NOT EXISTS idx_packaging_gtin ON product_packaging(gtin_packaging);

CREATE TABLE IF NOT EXISTS price_type (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    price_kind VARCHAR(20) NOT NULL CHECK (price_kind IN ('RETAIL', 'WHOLESALE', 'PURCHASE', 'DISCOUNT')),
    includes_vat BOOLEAN DEFAULT TRUE,
    currency VARCHAR(3) DEFAULT 'RUB',
    is_default BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, code)
);

CREATE TABLE IF NOT EXISTS product_price (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE CASCADE,
    price_type_id BIGINT NOT NULL REFERENCES price_type(id) ON DELETE CASCADE,
    price DECIMAL(15,2) NOT NULL CHECK (price >= 0),
    valid_from DATE DEFAULT CURRENT_DATE,
    valid_to DATE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (product_id, price_type_id, valid_from)
);
CREATE INDEX IF NOT EXISTS idx_price_product ON product_price(product_id);
