-- Этап 6: склад минимум + заказы (по sql/01: counterparty/warehouse/receipt_*/warehouse_balance/sales_order*/shipment_*).
-- Упрощения: responsible/manager → users (не employee), без партий/упаковок/маркировочных массивов
-- (остатки в разрезе склад×товар), перемещения и инвентаризации — следующим слоем.

CREATE TABLE IF NOT EXISTS counterparty (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    inn VARCHAR(12) NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    short_name VARCHAR(100),
    phone VARCHAR(20),
    email VARCHAR(100),
    is_supplier BOOLEAN DEFAULT FALSE,
    is_buyer BOOLEAN DEFAULT FALSE,
    credit_limit DECIMAL(15,2) DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, inn)
);

CREATE TABLE IF NOT EXISTS warehouse (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    warehouse_type VARCHAR(20) DEFAULT 'MAIN'
        CHECK (warehouse_type IN ('MAIN','STORAGE','SHOP','TRANSIT')),
    manager_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, code)
);

-- Касса привязывается к складу для списания остатков (nullable = без контроля).
ALTER TABLE cash_register ADD COLUMN IF NOT EXISTS warehouse_id BIGINT REFERENCES warehouse(id) ON DELETE SET NULL;

-- Поступление от поставщика.
CREATE TABLE IF NOT EXISTS receipt_document (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES counterparty(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    document_number VARCHAR(50) NOT NULL,
    document_date DATE NOT NULL DEFAULT CURRENT_DATE,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat DECIMAL(15,2) DEFAULT 0,
    is_posted BOOLEAN DEFAULT FALSE,
    posted_at TIMESTAMPTZ,
    responsible_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    comment TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, document_number)
);

CREATE TABLE IF NOT EXISTS receipt_line (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES receipt_document(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    quantity DECIMAL(15,3) NOT NULL CHECK (quantity > 0),
    price DECIMAL(15,2) NOT NULL CHECK (price >= 0),
    vat_rate DECIMAL(5,2) NOT NULL DEFAULT 20.00
);

-- Остатки в разрезе склад × товар.
CREATE TABLE IF NOT EXISTS warehouse_balance (
    id BIGSERIAL PRIMARY KEY,
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    quantity DECIMAL(15,3) NOT NULL DEFAULT 0,
    reserved_quantity DECIMAL(15,3) NOT NULL DEFAULT 0,
    last_updated TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (warehouse_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_balance_product ON warehouse_balance(product_id);

-- Заказ покупателя.
CREATE TABLE IF NOT EXISTS sales_order (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    buyer_id BIGINT NOT NULL REFERENCES counterparty(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    order_number VARCHAR(50) NOT NULL,
    order_date DATE NOT NULL DEFAULT CURRENT_DATE,
    order_type VARCHAR(20) DEFAULT 'RETAIL'
        CHECK (order_type IN ('RETAIL','WHOLESALE','ONLINE','PREORDER')),
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat DECIMAL(15,2) DEFAULT 0,
    status VARCHAR(30) DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','CONFIRMED','SHIPPED','COMPLETED','CANCELED')),
    delivery_address TEXT,
    manager_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    comment TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, order_number)
);

CREATE TABLE IF NOT EXISTS sales_order_line (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES sales_order(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    quantity DECIMAL(15,3) NOT NULL CHECK (quantity > 0),
    price DECIMAL(15,2) NOT NULL CHECK (price >= 0),
    vat_rate DECIMAL(5,2) NOT NULL DEFAULT 20.00,
    discount DECIMAL(15,2) DEFAULT 0,
    reserved_quantity DECIMAL(15,3) DEFAULT 0
);

-- Отгрузка (может быть частичной, несколько отгрузок на заказ).
CREATE TABLE IF NOT EXISTS shipment_document (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    buyer_id BIGINT NOT NULL REFERENCES counterparty(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    sales_order_id BIGINT REFERENCES sales_order(id) ON DELETE SET NULL,
    document_number VARCHAR(50) NOT NULL,
    document_date DATE NOT NULL DEFAULT CURRENT_DATE,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat DECIMAL(15,2) DEFAULT 0,
    is_posted BOOLEAN DEFAULT FALSE,
    posted_at TIMESTAMPTZ,
    responsible_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    comment TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, document_number)
);

CREATE TABLE IF NOT EXISTS shipment_line (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES shipment_document(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    order_line_id BIGINT REFERENCES sales_order_line(id) ON DELETE SET NULL,
    quantity DECIMAL(15,3) NOT NULL CHECK (quantity > 0),
    price DECIMAL(15,2) NOT NULL CHECK (price >= 0),
    vat_rate DECIMAL(5,2) NOT NULL DEFAULT 20.00
);
