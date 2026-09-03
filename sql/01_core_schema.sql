-- ============================================================
-- 01_core_schema.sql – Основная схема (полная версия)
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "btree_gin";

-- Организации
CREATE TABLE organization (
    id BIGSERIAL PRIMARY KEY,
    inn VARCHAR(12) UNIQUE NOT NULL,
    kpp VARCHAR(9) NOT NULL,
    ogrn VARCHAR(13),
    full_name VARCHAR(255) NOT NULL,
    short_name VARCHAR(100),
    legal_address TEXT NOT NULL,
    actual_address TEXT,
    okpo VARCHAR(10),
    okved VARCHAR(20),
    tax_system VARCHAR(20) DEFAULT 'OSN' CHECK (tax_system IN ('OSN', 'USN', 'ESHN', 'UTII', 'PSN')),
    bank_name VARCHAR(100),
    bic VARCHAR(9),
    correspondent_account VARCHAR(20),
    settlement_account VARCHAR(20),
    phone VARCHAR(20),
    email VARCHAR(100),
    director_name VARCHAR(100),
    chief_accountant VARCHAR(100),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Контрагенты
CREATE TABLE counterparty (
    id BIGSERIAL PRIMARY KEY,
    inn VARCHAR(12) UNIQUE NOT NULL,
    kpp VARCHAR(9),
    full_name VARCHAR(255) NOT NULL,
    short_name VARCHAR(100),
    legal_address TEXT,
    actual_address TEXT,
    is_supplier BOOLEAN DEFAULT FALSE,
    is_buyer BOOLEAN DEFAULT FALSE,
    is_commission_agent BOOLEAN DEFAULT FALSE,
    is_registered_in_gis_mt BOOLEAN DEFAULT FALSE,
    gis_mt_registration_date DATE,
    bank_name VARCHAR(100),
    bic VARCHAR(9),
    settlement_account VARCHAR(20),
    phone VARCHAR(20),
    email VARCHAR(100),
    contact_person VARCHAR(100),
    credit_limit DECIMAL(15,2) DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Сотрудники (базовые поля, без user_id – он добавится позже)
CREATE TABLE employee (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    last_name VARCHAR(50) NOT NULL,
    first_name VARCHAR(50) NOT NULL,
    middle_name VARCHAR(50),
    tin VARCHAR(12),
    snils VARCHAR(14),
    position VARCHAR(100),
    department VARCHAR(100),
    is_cashier BOOLEAN DEFAULT FALSE,
    cashier_number VARCHAR(20),
    phone VARCHAR(20),
    email VARCHAR(100),
    is_active BOOLEAN DEFAULT TRUE,
    hired_date DATE,
    fired_date DATE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Склады
CREATE TABLE warehouse (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    code VARCHAR(20) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    address TEXT NOT NULL,
    warehouse_type VARCHAR(20) DEFAULT 'MAIN' CHECK (warehouse_type IN ('MAIN', 'STORAGE', 'SHOP', 'TRANSIT')),
    manager_id BIGINT REFERENCES employee(id),
    has_address_storage BOOLEAN DEFAULT FALSE,
    latitude DECIMAL(10,7),
    longitude DECIMAL(10,7),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Зоны склада
CREATE TABLE warehouse_zone (
    id BIGSERIAL PRIMARY KEY,
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    zone_type VARCHAR(20) DEFAULT 'STORAGE' CHECK (zone_type IN ('STORAGE', 'PICKING', 'QUARANTINE', 'REJECT', 'EXPEDITION')),
    max_weight DECIMAL(10,2),
    max_volume DECIMAL(10,2),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (warehouse_id, code)
);

-- Категории товаров
CREATE TABLE catalog_category (
    id BIGSERIAL PRIMARY KEY,
    parent_id BIGINT REFERENCES catalog_category(id),
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    full_path VARCHAR(255),
    tnved_code VARCHAR(10),
    okpd2_code VARCHAR(20),
    is_marked_by_default BOOLEAN DEFAULT FALSE,
    sort_order INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Бренды
CREATE TABLE catalog_brand (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    country VARCHAR(50),
    supplier_id BIGINT REFERENCES counterparty(id),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Товары
CREATE TABLE catalog_product (
    id BIGSERIAL PRIMARY KEY,
    sku VARCHAR(50) UNIQUE NOT NULL,
    supplier_sku VARCHAR(50),
    gtin VARCHAR(14) UNIQUE,
    name VARCHAR(255) NOT NULL,
    full_name TEXT,
    description TEXT,
    category_id INT REFERENCES catalog_category(id),
    brand_id INT REFERENCES catalog_brand(id),
    measure_unit VARCHAR(10) DEFAULT 'шт',
    packaging_measure VARCHAR(10) DEFAULT 'шт',
    net_weight DECIMAL(10,3),
    gross_weight DECIMAL(10,3),
    volume DECIMAL(10,3),
    base_price DECIMAL(15,2),
    vat_rate DECIMAL(5,2) DEFAULT 20.00,
    excise_rate DECIMAL(15,2) DEFAULT 0,
    is_marked BOOLEAN DEFAULT FALSE,
    marking_type VARCHAR(20),
    marking_product_group VARCHAR(50),
    is_alcohol BOOLEAN DEFAULT FALSE,
    alcohol_volume DECIMAL(5,2),
    alcohol_class VARCHAR(10),
    has_expiry_date BOOLEAN DEFAULT FALSE,
    shelf_life_days INT,
    has_serial_numbers BOOLEAN DEFAULT FALSE,
    has_batches BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Упаковки товаров
CREATE TABLE product_packaging (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE CASCADE,
    packaging_type VARCHAR(20) NOT NULL CHECK (packaging_type IN ('UNIT', 'GROUP', 'TRANSPORT', 'OSU')),
    gtin_packaging VARCHAR(14) NOT NULL,
    units_in_pack INT NOT NULL DEFAULT 1,
    width DECIMAL(10,2),
    height DECIMAL(10,2),
    depth DECIMAL(10,2),
    weight DECIMAL(10,3),
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (product_id, gtin_packaging)
);

-- Типы цен
CREATE TABLE price_type (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    code VARCHAR(20) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    price_type VARCHAR(20) NOT NULL CHECK (price_type IN ('RETAIL', 'WHOLESALE', 'PURCHASE', 'DISCOUNT')),
    includes_vat BOOLEAN DEFAULT TRUE,
    currency VARCHAR(3) DEFAULT 'RUB',
    is_default BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Цены товаров
CREATE TABLE product_price (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE CASCADE,
    price_type_id BIGINT NOT NULL REFERENCES price_type(id),
    price DECIMAL(15,2) NOT NULL,
    valid_from DATE DEFAULT CURRENT_DATE,
    valid_to DATE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (product_id, price_type_id, valid_from)
);

-- Партии товара
CREATE TABLE product_batch (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    supplier_id BIGINT REFERENCES counterparty(id),
    batch_number VARCHAR(50) NOT NULL,
    production_date DATE,
    expiry_date DATE,
    certificate_number VARCHAR(50),
    certificate_date DATE,
    marking_batch_id VARCHAR(100),
    receipt_document_id BIGINT,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (product_id, batch_number)
);

-- Коды маркировки
CREATE TABLE marking_code_pool (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    code VARCHAR(150) UNIQUE NOT NULL,
    code_hash VARCHAR(64) GENERATED ALWAYS AS (MD5(code)) STORED,
    gtin VARCHAR(14) NOT NULL,
    serial_number VARCHAR(20),
    verification_code VARCHAR(20),
    packaging_type VARCHAR(20) NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN (
        'AVAILABLE', 'IN_ORDER', 'SHIPPED', 'SOLD',
        'RETURNED', 'WRITTEN_OFF', 'RECEIVED', 'TRANSFERRED'
    )),
    supplier_id BIGINT REFERENCES counterparty(id),
    owner_organization_id BIGINT REFERENCES organization(id),
    receipt_document_id BIGINT,
    shipment_document_id BIGINT,
    sales_receipt_id BIGINT,
    received_at TIMESTAMP DEFAULT NOW(),
    sold_at TIMESTAMP,
    expiry_date DATE,
    alcohol_declaration_id BIGINT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Пачки маркировки
CREATE TABLE marking_batch (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    batch_number VARCHAR(50) UNIQUE NOT NULL,
    gis_mt_batch_id VARCHAR(100),
    total_codes INT NOT NULL,
    used_codes INT DEFAULT 0,
    available_codes INT GENERATED ALWAYS AS (total_codes - used_codes) STORED,
    receipt_document_id BIGINT,
    status VARCHAR(30) DEFAULT 'CREATED' CHECK (status IN ('CREATED', 'UPLOADED', 'CONFIRMED', 'CLOSED')),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Заказ поставщику
CREATE TABLE purchase_order (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    supplier_id BIGINT NOT NULL REFERENCES counterparty(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    order_number VARCHAR(50) UNIQUE NOT NULL,
    order_date DATE NOT NULL DEFAULT CURRENT_DATE,
    delivery_date DATE,
    currency VARCHAR(3) DEFAULT 'RUB',
    exchange_rate DECIMAL(10,4) DEFAULT 1,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat DECIMAL(15,2) DEFAULT 0,
    status VARCHAR(30) DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'APPROVED', 'SENT', 'DELIVERED_PARTIAL', 'DELIVERED', 'CANCELED')),
    responsible_id BIGINT REFERENCES employee(id),
    comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Поступление товаров
CREATE TABLE receipt_document (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    supplier_id BIGINT NOT NULL REFERENCES counterparty(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    document_number VARCHAR(50) UNIQUE NOT NULL,
    document_date DATE NOT NULL DEFAULT CURRENT_DATE,
    purchase_order_id BIGINT REFERENCES purchase_order(id),
    edi_invoice_number VARCHAR(50),
    edi_invoice_date DATE,
    edi_file_id VARCHAR(100),
    waybill_number VARCHAR(50),
    waybill_date DATE,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat DECIMAL(15,2) DEFAULT 0,
    total_without_vat DECIMAL(15,2) DEFAULT 0,
    is_posted BOOLEAN DEFAULT FALSE,
    posted_at TIMESTAMP,
    responsible_id BIGINT REFERENCES employee(id),
    comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Строки поступления
CREATE TABLE receipt_line (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES receipt_document(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    packaging_type VARCHAR(20) NOT NULL,
    quantity DECIMAL(15,3) NOT NULL,
    price DECIMAL(15,2) NOT NULL,
    vat_rate DECIMAL(5,2) NOT NULL DEFAULT 20.00,
    vat_amount DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity * vat_rate / 100) STORED,
    total_sum DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity) STORED,
    total_with_vat DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity * (1 + vat_rate / 100)) STORED,
    batch_id BIGINT REFERENCES product_batch(id),
    expiry_date DATE,
    marking_code_ids BIGINT[],
    purchase_order_line_id BIGINT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Остатки на складах
CREATE TABLE warehouse_balance (
    id BIGSERIAL PRIMARY KEY,
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    quantity DECIMAL(15,3) NOT NULL DEFAULT 0,
    reserved_quantity DECIMAL(15,3) NOT NULL DEFAULT 0,
    batch_id BIGINT REFERENCES product_batch(id),
    packaging_type VARCHAR(20),
    last_updated TIMESTAMP DEFAULT NOW(),
    UNIQUE (warehouse_id, product_id, packaging_type, batch_id)
);

-- Документы перемещения
CREATE TABLE movement_document (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    document_number VARCHAR(50) UNIQUE NOT NULL,
    document_date DATE NOT NULL DEFAULT CURRENT_DATE,
    from_warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    to_warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    movement_type VARCHAR(20) DEFAULT 'INTERNAL' CHECK (movement_type IN ('INTERNAL', 'TRANSFER', 'RETURN')),
    base_document_type VARCHAR(50),
    base_document_id BIGINT,
    responsible_id BIGINT REFERENCES employee(id),
    status VARCHAR(30) DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED', 'CANCELED', 'COMPLETED')),
    comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Строки перемещения
CREATE TABLE movement_line (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES movement_document(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    quantity DECIMAL(15,3) NOT NULL,
    packaging_type VARCHAR(20),
    batch_id BIGINT REFERENCES product_batch(id),
    marking_code_ids BIGINT[],
    created_at TIMESTAMP DEFAULT NOW()
);

-- Инвентаризация
CREATE TABLE inventory_document (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    document_number VARCHAR(50) UNIQUE NOT NULL,
    document_date DATE NOT NULL DEFAULT CURRENT_DATE,
    inventory_start_date DATE NOT NULL,
    inventory_end_date DATE NOT NULL,
    responsible_id BIGINT REFERENCES employee(id),
    commission_members BIGINT[],
    has_discrepancy BOOLEAN DEFAULT FALSE,
    total_surplus DECIMAL(15,2) DEFAULT 0,
    total_shortage DECIMAL(15,2) DEFAULT 0,
    status VARCHAR(30) DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED', 'CANCELED')),
    comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Строки инвентаризации
CREATE TABLE inventory_line (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES inventory_document(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    accounting_quantity DECIMAL(15,3) NOT NULL,
    accounting_sum DECIMAL(15,2) NOT NULL,
    actual_quantity DECIMAL(15,3) NOT NULL,
    actual_sum DECIMAL(15,2) NOT NULL,
    difference_quantity DECIMAL(15,3) GENERATED ALWAYS AS (actual_quantity - accounting_quantity) STORED,
    difference_sum DECIMAL(15,2) GENERATED ALWAYS AS (actual_sum - accounting_sum) STORED,
    discrepancy_reason VARCHAR(255),
    marking_code_ids BIGINT[],
    created_at TIMESTAMP DEFAULT NOW()
);

-- Заказ покупателя
CREATE TABLE sales_order (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    buyer_id BIGINT NOT NULL REFERENCES counterparty(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    order_number VARCHAR(50) UNIQUE NOT NULL,
    order_date DATE NOT NULL DEFAULT CURRENT_DATE,
    order_type VARCHAR(20) DEFAULT 'RETAIL' CHECK (order_type IN ('RETAIL', 'WHOLESALE', 'ONLINE', 'PREORDER')),
    contract_number VARCHAR(50),
    contract_date DATE,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat DECIMAL(15,2) DEFAULT 0,
    total_discount DECIMAL(15,2) DEFAULT 0,
    status VARCHAR(30) DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'CONFIRMED', 'PICKING', 'SHIPPED', 'COMPLETED', 'CANCELED')),
    delivery_address TEXT,
    delivery_date DATE,
    manager_id BIGINT REFERENCES employee(id),
    comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Строки заказа покупателя
CREATE TABLE sales_order_line (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES sales_order(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    quantity DECIMAL(15,3) NOT NULL,
    price DECIMAL(15,2) NOT NULL,
    vat_rate DECIMAL(5,2) NOT NULL DEFAULT 20.00,
    discount DECIMAL(15,2) DEFAULT 0,
    total_sum DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity - discount) STORED,
    reserved_quantity DECIMAL(15,3) DEFAULT 0,
    marking_code_ids BIGINT[],
    created_at TIMESTAMP DEFAULT NOW()
);

-- Отгрузка
CREATE TABLE shipment_document (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    buyer_id BIGINT NOT NULL REFERENCES counterparty(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    document_number VARCHAR(50) UNIQUE NOT NULL,
    document_date DATE NOT NULL DEFAULT CURRENT_DATE,
    sales_order_id BIGINT REFERENCES sales_order(id),
    edi_invoice_number VARCHAR(50),
    edi_invoice_date DATE,
    waybill_number VARCHAR(50),
    waybill_date DATE,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat DECIMAL(15,2) DEFAULT 0,
    total_without_vat DECIMAL(15,2) DEFAULT 0,
    is_posted BOOLEAN DEFAULT FALSE,
    posted_at TIMESTAMP,
    responsible_id BIGINT REFERENCES employee(id),
    comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Строки отгрузки
CREATE TABLE shipment_line (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES shipment_document(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    packaging_type VARCHAR(20),
    quantity DECIMAL(15,3) NOT NULL,
    price DECIMAL(15,2) NOT NULL,
    vat_rate DECIMAL(5,2) NOT NULL DEFAULT 20.00,
    total_sum DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity) STORED,
    batch_id BIGINT REFERENCES product_batch(id),
    marking_code_ids BIGINT[],
    order_line_id BIGINT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- ККТ
CREATE TABLE cash_register (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    reg_number VARCHAR(20) UNIQUE NOT NULL,
    factory_number VARCHAR(30) NOT NULL,
    model VARCHAR(50) NOT NULL,
    fn_factory_number VARCHAR(30) NOT NULL,
    fn_reg_number VARCHAR(20),
    fn_serial_number VARCHAR(30),
    ofd_name VARCHAR(100),
    ofd_inn VARCHAR(12),
    ofd_url VARCHAR(255),
    fn_install_date DATE,
    fn_expiry_date DATE,
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'BLOCKED', 'REPLACED')),
    installation_address TEXT,
    warehouse_id BIGINT REFERENCES warehouse(id),
    responsible_id BIGINT REFERENCES employee(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Кассовые смены
CREATE TABLE cash_shift (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    cash_register_id BIGINT NOT NULL REFERENCES cash_register(id),
    shift_number INT NOT NULL,
    shift_date DATE NOT NULL DEFAULT CURRENT_DATE,
    opened_at TIMESTAMP NOT NULL DEFAULT NOW(),
    opened_by_id BIGINT REFERENCES employee(id),
    closed_at TIMESTAMP,
    closed_by_id BIGINT REFERENCES employee(id),
    start_cash DECIMAL(15,2) DEFAULT 0,
    cash_sales DECIMAL(15,2) DEFAULT 0,
    card_sales DECIMAL(15,2) DEFAULT 0,
    total_sales DECIMAL(15,2) GENERATED ALWAYS AS (cash_sales + card_sales) STORED,
    cash_returns DECIMAL(15,2) DEFAULT 0,
    card_returns DECIMAL(15,2) DEFAULT 0,
    expected_cash DECIMAL(15,2),
    actual_cash DECIMAL(15,2),
    discrepancy DECIMAL(15,2) GENERATED ALWAYS AS (actual_cash - expected_cash) STORED,
    status VARCHAR(20) DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'CLOSED', 'CANCELED')),
    x_report_id BIGINT,
    z_report_id BIGINT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Кассовый чек
CREATE TABLE sales_receipt (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    receipt_number VARCHAR(20) NOT NULL,
    shift_number INT NOT NULL,
    cash_register_id BIGINT REFERENCES cash_register(id),
    cashier_id BIGINT REFERENCES employee(id),
    receipt_type VARCHAR(20) DEFAULT 'SALE' CHECK (receipt_type IN ('SALE', 'RETURN', 'CORRECTION')),
    sales_order_id BIGINT REFERENCES sales_order(id),
    fiscal_drive_number VARCHAR(20),
    fiscal_document_number VARCHAR(20),
    fiscal_sign VARCHAR(30),
    fiscal_document_date TIMESTAMP,
    fns_site_url VARCHAR(255),
    qr_code_url VARCHAR(255),
    total_amount DECIMAL(15,2) NOT NULL,
    total_vat DECIMAL(15,2) DEFAULT 0,
    total_discount DECIMAL(15,2) DEFAULT 0,
    payment_type VARCHAR(20) NOT NULL CHECK (payment_type IN ('CASH', 'CARD', 'MIXED', 'PREPAYMENT', 'CREDIT')),
    payment_cash DECIMAL(15,2) DEFAULT 0,
    payment_card DECIMAL(15,2) DEFAULT 0,
    change_amount DECIMAL(15,2) DEFAULT 0,
    has_marked_goods BOOLEAN DEFAULT FALSE,
    has_alcohol BOOLEAN DEFAULT FALSE,
    is_uploaded_to_fns BOOLEAN DEFAULT FALSE,
    uploaded_at TIMESTAMP,
    fns_response_code VARCHAR(10),
    fns_response_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Товары в чеке
CREATE TABLE sales_receipt_item (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES sales_receipt(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    product_name VARCHAR(255) NOT NULL,
    product_sku VARCHAR(50),
    packaging_type VARCHAR(20),
    quantity DECIMAL(15,3) NOT NULL,
    price DECIMAL(15,2) NOT NULL,
    vat_rate DECIMAL(5,2) NOT NULL,
    vat_amount DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity * vat_rate / 100) STORED,
    total_amount DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity) STORED,
    discount DECIMAL(15,2) DEFAULT 0,
    is_marked BOOLEAN DEFAULT FALSE,
    marking_code_ids BIGINT[],
    alcohol_volume DECIMAL(5,2),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Связь чека с кодами маркировки
CREATE TABLE receipt_marking_link (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES sales_receipt(id) ON DELETE CASCADE,
    marking_code_id BIGINT NOT NULL REFERENCES marking_code_pool(id),
    scanned_at TIMESTAMP DEFAULT NOW(),
    cashier_id BIGINT REFERENCES employee(id),
    UNIQUE (marking_code_id)
);

-- Книга покупок
CREATE TABLE purchase_book (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    quarter INT NOT NULL CHECK (quarter BETWEEN 1 AND 4),
    year INT NOT NULL,
    entry_number INT NOT NULL,
    document_type VARCHAR(50),
    document_number VARCHAR(50),
    document_date DATE,
    supplier_id BIGINT REFERENCES counterparty(id),
    supplier_inn VARCHAR(12),
    supplier_kpp VARCHAR(9),
    purchase_amount DECIMAL(15,2),
    vat_amount DECIMAL(15,2),
    total_amount DECIMAL(15,2),
    vat_rate DECIMAL(5,2),
    invoice_number VARCHAR(50),
    invoice_date DATE,
    accounting_date DATE,
    is_import BOOLEAN DEFAULT FALSE,
    is_tax_agent BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Книга продаж
CREATE TABLE sales_book (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    quarter INT NOT NULL CHECK (quarter BETWEEN 1 AND 4),
    year INT NOT NULL,
    entry_number INT NOT NULL,
    document_type VARCHAR(50),
    document_number VARCHAR(50),
    document_date DATE,
    buyer_id BIGINT REFERENCES counterparty(id),
    buyer_inn VARCHAR(12),
    buyer_kpp VARCHAR(9),
    sales_amount DECIMAL(15,2),
    vat_amount DECIMAL(15,2),
    total_amount DECIMAL(15,2),
    vat_rate DECIMAL(5,2),
    invoice_number VARCHAR(50),
    invoice_date DATE,
    is_export BOOLEAN DEFAULT FALSE,
    is_commission BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Регистры налогового учета
CREATE TABLE tax_accounting_register (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    period_year INT NOT NULL,
    period_month INT CHECK (period_month BETWEEN 1 AND 12),
    register_type VARCHAR(50) NOT NULL CHECK (register_type IN (
        'INCOME', 'EXPENSES', 'VAT_INPUT', 'VAT_OUTPUT',
        'PROPERTY', 'PAYROLL', 'INSURANCE', 'OTHER'
    )),
    base_document_type VARCHAR(50),
    base_document_id BIGINT,
    base_document_number VARCHAR(50),
    base_document_date DATE,
    amount DECIMAL(15,2) NOT NULL,
    vat_amount DECIMAL(15,2) DEFAULT 0,
    comment TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Декларационные регистры
CREATE TABLE declaration_register (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    report_year INT NOT NULL,
    report_quarter INT CHECK (report_quarter BETWEEN 1 AND 4),
    declaration_type VARCHAR(50) NOT NULL CHECK (declaration_type IN (
        'VAT', 'INCOME_TAX', 'USN', 'PROPERTY_TAX', 'INSURANCE'
    )),
    line_number VARCHAR(20) NOT NULL,
    line_description TEXT,
    value DECIMAL(15,2) NOT NULL,
    source_register_type VARCHAR(50),
    source_register_id BIGINT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Индексы (основные)
CREATE INDEX idx_org_inn ON organization(inn);
CREATE INDEX idx_cp_inn ON counterparty(inn);
CREATE INDEX idx_employee_org ON employee(organization_id);
CREATE INDEX idx_warehouse_org ON warehouse(organization_id);
CREATE INDEX idx_product_sku ON catalog_product(sku);
CREATE INDEX idx_product_gtin ON catalog_product(gtin);
CREATE INDEX idx_product_marked ON catalog_product(is_marked) WHERE is_marked = TRUE;
CREATE INDEX idx_marking_hash ON marking_code_pool(code_hash);
CREATE INDEX idx_marking_status ON marking_code_pool(status);
CREATE INDEX idx_receipt_supplier ON receipt_document(supplier_id);
CREATE INDEX idx_receipt_date ON receipt_document(document_date);
CREATE INDEX idx_balance_warehouse_product ON warehouse_balance(warehouse_id, product_id);
CREATE INDEX idx_shipment_buyer ON shipment_document(buyer_id);
CREATE INDEX idx_receipt_cashier ON sales_receipt(cashier_id);
CREATE INDEX idx_receipt_fiscal ON sales_receipt(fiscal_sign);
CREATE INDEX idx_receipt_date ON sales_receipt(created_at);
CREATE INDEX idx_pb_org ON purchase_book(organization_id);
CREATE INDEX idx_sb_org ON sales_book(organization_id);
-- дополнительные индексы можно добавить позже

-- Материализованные представления
CREATE MATERIALIZED VIEW mv_product_turnover AS
WITH receipts AS (
    SELECT rl.product_id, SUM(rl.quantity) AS receipt_quantity, SUM(rl.total_sum) AS receipt_sum
    FROM receipt_line rl JOIN receipt_document rd ON rl.document_id = rd.id
    WHERE rd.is_posted = TRUE GROUP BY rl.product_id
),
shipments AS (
    SELECT sl.product_id, SUM(sl.quantity) AS shipment_quantity, SUM(sl.total_sum) AS shipment_sum
    FROM shipment_line sl JOIN shipment_document sd ON sl.document_id = sd.id
    WHERE sd.is_posted = TRUE GROUP BY sl.product_id
),
sales AS (
    SELECT sri.product_id, SUM(sri.quantity) AS sale_quantity, SUM(sri.total_amount) AS sale_sum
    FROM sales_receipt_item sri JOIN sales_receipt sr ON sri.receipt_id = sr.id
    GROUP BY sri.product_id
)
SELECT
    p.id AS product_id, p.sku, p.name, p.gtin,
    COALESCE(r.receipt_quantity,0) AS receipt_quantity,
    COALESCE(r.receipt_sum,0) AS receipt_sum,
    COALESCE(sh.shipment_quantity,0) AS shipment_quantity,
    COALESCE(sh.shipment_sum,0) AS shipment_sum,
    COALESCE(s.sale_quantity,0) AS sale_quantity,
    COALESCE(s.sale_sum,0) AS sale_sum,
    (COALESCE(r.receipt_quantity,0) - COALESCE(sh.shipment_quantity,0) - COALESCE(s.sale_quantity,0)) AS balance_quantity
FROM catalog_product p
LEFT JOIN receipts r ON p.id = r.product_id
LEFT JOIN shipments sh ON p.id = sh.product_id
LEFT JOIN sales s ON p.id = s.product_id
WITH NO DATA;

CREATE UNIQUE INDEX idx_mv_product_turnover_id ON mv_product_turnover(product_id);

CREATE MATERIALIZED VIEW mv_marking_balance AS
SELECT p.id AS product_id, p.sku, p.name, p.gtin, mc.packaging_type, mc.status, COUNT(*) AS code_count
FROM marking_code_pool mc
JOIN catalog_product p ON mc.product_id = p.id
WHERE mc.status IN ('AVAILABLE', 'IN_ORDER')
GROUP BY p.id, p.sku, p.name, p.gtin, mc.packaging_type, mc.status
WITH NO DATA;

CREATE MATERIALIZED VIEW mv_counterparty_turnover AS
SELECT c.id AS counterparty_id, c.inn, c.short_name,
    COALESCE(SUM(r.total_amount),0) AS purchase_amount,
    COALESCE(COUNT(r.id),0) AS purchase_count,
    COALESCE(SUM(s.total_amount),0) AS sales_amount,
    COALESCE(COUNT(s.id),0) AS sales_count
FROM counterparty c
LEFT JOIN receipt_document r ON c.id = r.supplier_id AND r.is_posted = TRUE
LEFT JOIN shipment_document s ON c.id = s.buyer_id AND s.is_posted = TRUE
GROUP BY c.id, c.inn, c.short_name
WITH NO DATA;

-- Функции обновления
CREATE OR REPLACE FUNCTION refresh_mv_product_turnover() RETURNS VOID AS $$
BEGIN REFRESH MATERIALIZED VIEW CONCURRENTLY mv_product_turnover; END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION refresh_mv_marking_balance() RETURNS VOID AS $$
BEGIN REFRESH MATERIALIZED VIEW CONCURRENTLY mv_marking_balance; END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION refresh_mv_counterparty_turnover() RETURNS VOID AS $$
BEGIN REFRESH MATERIALIZED VIEW CONCURRENTLY mv_counterparty_turnover; END;
$$ LANGUAGE plpgsql;

-- Триггер для updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

-- Назначение триггеров (для таблиц с updated_at)
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename IN
        ('organization','counterparty','employee','warehouse','catalog_category','catalog_product',
         'product_packaging','price_type','product_price','purchase_order','receipt_document',
         'movement_document','inventory_document','sales_order','shipment_document','cash_register',
         'cash_shift','sales_receipt','marking_code_pool','marking_batch')
    LOOP
        EXECUTE format('CREATE TRIGGER update_%I_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();', t, t);
    END LOOP;
END;
$$;

-- Начальные данные (базовые категории)
INSERT INTO catalog_category (code, name, is_active) VALUES
('ROOT','Все товары',TRUE),
('FOOD','Продукты питания',TRUE),
('BEVERAGES','Напитки',TRUE),
('ALCOHOL','Алкогольная продукция',TRUE);
