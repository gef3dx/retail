-- ============================================================
-- 02_integrations.sql – Интеграции (ОФД, ЕГАИС, Маркетплейсы)
-- ============================================================

-- ============================================================
-- 1. ОФД (ОПЕРАТОР ФИСКАЛЬНЫХ ДАННЫХ)
-- ============================================================

-- 1.1. Настройки ОФД
CREATE TABLE ofd_settings (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Основные данные ОФД
    ofd_name VARCHAR(100) NOT NULL,
    ofd_inn VARCHAR(12) NOT NULL,
    ofd_reg_number VARCHAR(20),

    -- API настройки
    api_url VARCHAR(255) NOT NULL,
    api_key VARCHAR(255),
    api_login VARCHAR(100),
    api_password VARCHAR(255),

    -- Таймауты и настройки
    request_timeout INT DEFAULT 30000,
    max_retries INT DEFAULT 3,
    retry_interval INT DEFAULT 5000,

    -- Автоматическая отправка
    auto_send_enabled BOOLEAN DEFAULT TRUE,
    send_interval INT DEFAULT 60,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE ofd_settings IS 'Настройки подключения к ОФД';

-- 1.2. Статусы отправки чеков в ОФД
CREATE TABLE ofd_send_status (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES sales_receipt(id),
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Данные отправки
    send_attempt INT DEFAULT 0,
    send_timestamp TIMESTAMP,

    -- Статус от ОФД
    ofd_status_code VARCHAR(20),
    ofd_status_message TEXT,
    ofd_request_id VARCHAR(50),

    -- Ответ от ОФД
    ofd_response_json JSONB,
    ofd_response_datetime TIMESTAMP,

    -- Фискальные данные (заполняются после ответа ОФД)
    fiscal_document_number VARCHAR(20),
    fiscal_sign VARCHAR(30),
    fiscal_document_datetime TIMESTAMP,

    -- Ссылки
    fns_site_url VARCHAR(255),
    ofd_site_url VARCHAR(255),
    qr_code_url VARCHAR(255),

    -- Статус обработки
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'SENT', 'RECEIVED', 'COMPLETED', 'FAILED', 'RETRY'
    )),

    error_message TEXT,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE ofd_send_status IS 'Статусы отправки чеков в ОФД';

CREATE INDEX idx_ofd_receipt ON ofd_send_status(receipt_id);
CREATE INDEX idx_ofd_status ON ofd_send_status(status) WHERE status IN ('PENDING', 'RETRY');
CREATE INDEX idx_ofd_fiscal_sign ON ofd_send_status(fiscal_sign) WHERE fiscal_sign IS NOT NULL;

-- 1.3. Квитанции о регистрации ККТ в ОФД
CREATE TABLE ofd_registration (
    id BIGSERIAL PRIMARY KEY,
    cash_register_id BIGINT NOT NULL REFERENCES cash_register(id),
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Данные регистрации
    reg_request_json JSONB,
    reg_response_json JSONB,
    reg_number VARCHAR(20),
    reg_datetime TIMESTAMP,

    -- Статус
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'REGISTERED', 'REJECTED', 'EXPIRED')),

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE ofd_registration IS 'Регистрация ККТ в ОФД';

-- ============================================================
-- 2. ФС РАР (АЛКОГОЛЬ) - ЕГАИС
-- ============================================================

-- 2.1. Лицензии на алкоголь
CREATE TABLE alcohol_license (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Номер лицензии
    license_number VARCHAR(50) UNIQUE NOT NULL,
    license_series VARCHAR(20),

    -- Вид лицензии
    license_type VARCHAR(50) CHECK (license_type IN (
        'RETAIL_ALCOHOL', 'RETAIL_BEER', 'WHOLESALE_ALCOHOL', 'STORAGE_ALCOHOL'
    )),

    -- Территория действия
    region_code VARCHAR(10),
    address TEXT,

    -- Сроки
    issued_date DATE NOT NULL,
    expiry_date DATE NOT NULL,

    -- Приложение к лицензии (перечень точек)
    attachment_json JSONB,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE alcohol_license IS 'Лицензии на алкогольную продукцию';

-- 2.2. Алкогольные декларации
CREATE TABLE alcohol_declaration (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Период декларации
    declaration_year INT NOT NULL,
    declaration_quarter INT NOT NULL CHECK (declaration_quarter BETWEEN 1 AND 4),
    declaration_type VARCHAR(20) CHECK (declaration_type IN ('ALCOHOL', 'BEER', 'ETHYL')),

    -- Номер декларации
    declaration_number VARCHAR(50) UNIQUE NOT NULL,
    declaration_date DATE NOT NULL,

    -- Статус
    status VARCHAR(20) DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'CREATED', 'SENT', 'ACCEPTED', 'REJECTED', 'CANCELED')),

    -- Файлы
    xml_file_data BYTEA,
    xml_file_name VARCHAR(255),
    response_file_data BYTEA,
    response_file_name VARCHAR(255),

    -- Ответ от ФС РАР
    response_code VARCHAR(10),
    response_message TEXT,

    -- Данные для отправки
    sender_id VARCHAR(50),
    recipient_id VARCHAR(50),

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE alcohol_declaration IS 'Декларации по алкоголю (ФС РАР)';

CREATE INDEX idx_alcohol_dec_period ON alcohol_declaration(declaration_year, declaration_quarter);
CREATE INDEX idx_alcohol_dec_status ON alcohol_declaration(status);

-- 2.3. Разделы алкогольной декларации
CREATE TABLE alcohol_declaration_section (
    id BIGSERIAL PRIMARY KEY,
    declaration_id BIGINT NOT NULL REFERENCES alcohol_declaration(id) ON DELETE CASCADE,

    -- Номер раздела (1-9)
    section_number INT NOT NULL CHECK (section_number BETWEEN 1 AND 9),
    section_name VARCHAR(100),

    -- Данные раздела (JSON)
    section_data JSONB NOT NULL,

    -- Суммарные показатели
    total_volume DECIMAL(15,3),
    total_amount DECIMAL(15,2),
    total_excise DECIMAL(15,2),

    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE alcohol_declaration_section IS 'Разделы алкогольных деклараций';

-- 2.4. Акцизные марки (для списания)
CREATE TABLE alcohol_excise_stamp (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),

    -- Серия и номер марки
    stamp_series VARCHAR(10) NOT NULL,
    stamp_number VARCHAR(20) NOT NULL,
    full_stamp_code VARCHAR(50) UNIQUE NOT NULL,

    -- Тип марки
    stamp_type VARCHAR(30) CHECK (stamp_type IN ('FEDERAL', 'REGIONAL')),

    -- Статус
    status VARCHAR(20) DEFAULT 'AVAILABLE' CHECK (status IN (
        'AVAILABLE', 'STICKED', 'SOLD', 'RETURNED', 'WRITTEN_OFF', 'DESTROYED'
    )),

    -- Связь с кодом маркировки (все маркированные бутылки имеют код)
    marking_code_id BIGINT REFERENCES marking_code_pool(id),

    -- Документы движения
    receipt_document_id BIGINT,
    sales_receipt_id BIGINT,

    -- Дата наклейки
    sticked_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE alcohol_excise_stamp IS 'Акцизные марки для алкоголя';

CREATE INDEX idx_stamp_code ON alcohol_excise_stamp(full_stamp_code);
CREATE INDEX idx_stamp_status ON alcohol_excise_stamp(status);
CREATE INDEX idx_stamp_marking ON alcohol_excise_stamp(marking_code_id);

-- 2.5. Алкогольный журнал учета (форма 12)
CREATE TABLE alcohol_journal (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Период
    journal_year INT NOT NULL,
    journal_month INT NOT NULL CHECK (journal_month BETWEEN 1 AND 12),

    -- Запись
    entry_number INT NOT NULL,
    entry_date DATE NOT NULL,

    -- Тип операции
    operation_type VARCHAR(50) CHECK (operation_type IN (
        'RECEIPT', 'SALE', 'RETURN', 'WRITE_OFF', 'INVENTORY'
    )),

    -- Продукция
    product_id BIGINT REFERENCES catalog_product(id),
    product_name VARCHAR(255),

    -- Количество (в дал)
    quantity DECIMAL(15,3) NOT NULL,
    quantity_in_bottles INT,

    -- Акциз
    excise_rate DECIMAL(15,2),
    excise_amount DECIMAL(15,2),

    -- Документ-основание
    base_document_type VARCHAR(50),
    base_document_number VARCHAR(50),
    base_document_date DATE,

    -- Контрагент (для ЕГАИС)
    counterparty_inn VARCHAR(12),
    counterparty_name VARCHAR(255),

    -- Статус
    is_verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE alcohol_journal IS 'Журнал учета алкогольной продукции (форма 12)';

CREATE INDEX idx_alcohol_journal_period ON alcohol_journal(journal_year, journal_month);
CREATE INDEX idx_alcohol_journal_product ON alcohol_journal(product_id);

-- 2.6. Связь с ЕГАИС (настройки)
CREATE TABLE egais_settings (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- ЕГАИС конфигурация
    egais_version VARCHAR(10) DEFAULT '3.0',

    -- Сертификаты
    certificate_serial VARCHAR(50),
    certificate_thumbprint VARCHAR(100),
    certificate_file BYTEA,
    certificate_password VARCHAR(255),

    -- Адреса сервисов
    fsrar_api_url VARCHAR(255),
    fsrar_wsdl_url VARCHAR(255),

    -- УТМ (Универсальный транспортный модуль)
    utm_host VARCHAR(100),
    utm_port INT DEFAULT 8080,
    utm_username VARCHAR(50),
    utm_password VARCHAR(255),

    -- Настройки
    auto_send_declaration BOOLEAN DEFAULT FALSE,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE egais_settings IS 'Настройки ЕГАИС';

-- ============================================================
-- 3. МАРКЕТПЛЕЙСЫ
-- ============================================================

-- 3.1. Настройки маркетплейсов
CREATE TABLE marketplace_settings (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Идентификация маркетплейса
    marketplace_code VARCHAR(50) NOT NULL CHECK (marketplace_code IN (
        'OZON', 'WILDBERRIES', 'YANDEX_MARKET', 'MEGAMARKET',
        'SBERMEGAMARKET', 'GOODS', 'ALIEXPRESS', 'BERU', 'YANDEX_LAVKA'
    )),

    -- API настройки
    api_url VARCHAR(255) NOT NULL,
    api_key VARCHAR(255),
    client_id VARCHAR(100),
    client_secret VARCHAR(255),
    access_token TEXT,
    token_expires_at TIMESTAMP,

    -- Идентификаторы магазина
    seller_id VARCHAR(50),
    warehouse_id VARCHAR(50),
    campaign_id VARCHAR(50),

    -- Ставки и комиссии
    commission_rate DECIMAL(5,2) DEFAULT 0,
    delivery_commission DECIMAL(5,2) DEFAULT 0,

    -- Настройки синхронизации
    sync_orders_enabled BOOLEAN DEFAULT TRUE,
    sync_stocks_enabled BOOLEAN DEFAULT TRUE,
    sync_prices_enabled BOOLEAN DEFAULT TRUE,
    sync_interval INT DEFAULT 300,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (organization_id, marketplace_code)
);

COMMENT ON TABLE marketplace_settings IS 'Настройки подключения к маркетплейсам';

-- 3.2. Заказы с маркетплейсов
CREATE TABLE marketplace_order (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    marketplace_settings_id BIGINT NOT NULL REFERENCES marketplace_settings(id),

    -- Внешний идентификатор
    external_order_id VARCHAR(100) NOT NULL,
    external_created_at TIMESTAMP,

    -- Данные заказа
    order_number VARCHAR(50) UNIQUE NOT NULL,
    order_date TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Покупатель
    buyer_name VARCHAR(255),
    buyer_phone VARCHAR(20),
    buyer_email VARCHAR(100),
    buyer_address TEXT,

    -- Суммы
    total_amount DECIMAL(15,2) NOT NULL,
    delivery_amount DECIMAL(15,2) DEFAULT 0,
    discount_amount DECIMAL(15,2) DEFAULT 0,

    -- Валюта
    currency VARCHAR(3) DEFAULT 'RUB',

    -- Статус заказа на маркетплейсе
    marketplace_status VARCHAR(50),
    marketplace_status_updated_at TIMESTAMP,

    -- Внутренний статус
    status VARCHAR(30) DEFAULT 'NEW' CHECK (status IN (
        'NEW', 'ACCEPTED', 'PICKING', 'READY_TO_SHIP', 'SHIPPED', 'DELIVERED', 'CANCELED', 'RETURNED'
    )),

    -- Способ доставки
    delivery_service VARCHAR(100),
    delivery_tracking_number VARCHAR(100),

    -- Связь с внутренним заказом
    sales_order_id BIGINT REFERENCES sales_order(id),

    -- Данные маркетплейса (JSON)
    raw_data JSONB,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (marketplace_settings_id, external_order_id)
);

COMMENT ON TABLE marketplace_order IS 'Заказы с маркетплейсов';

CREATE INDEX idx_mp_order_ext ON marketplace_order(external_order_id);
CREATE INDEX idx_mp_order_status ON marketplace_order(status);
CREATE INDEX idx_mp_order_date ON marketplace_order(order_date);

-- 3.3. Товары в заказах маркетплейсов
CREATE TABLE marketplace_order_item (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES marketplace_order(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),

    -- Внешний ID товара на маркетплейсе
    external_product_id VARCHAR(100),
    external_offer_id VARCHAR(100),

    -- Данные товара
    product_name VARCHAR(255) NOT NULL,
    product_sku VARCHAR(50),
    quantity INT NOT NULL,
    price DECIMAL(15,2) NOT NULL,
    total_sum DECIMAL(15,2) GENERATED ALWAYS AS (price * quantity) STORED,

    -- Связь с внутренним заказом
    sales_order_line_id BIGINT,

    -- Маркировка
    marking_code_ids BIGINT[],

    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE marketplace_order_item IS 'Товары в заказах маркетплейсов';

CREATE INDEX idx_mp_order_item_order ON marketplace_order_item(order_id);
CREATE INDEX idx_mp_order_item_product ON marketplace_order_item(product_id);

-- 3.4. Отгрузки на маркетплейсы
CREATE TABLE marketplace_shipment (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    marketplace_settings_id BIGINT NOT NULL REFERENCES marketplace_settings(id),
    sales_order_id BIGINT REFERENCES sales_order(id),

    -- Внешние ID
    external_shipment_id VARCHAR(100),
    external_order_id VARCHAR(100),

    -- Номер отгрузки
    shipment_number VARCHAR(50) UNIQUE NOT NULL,
    shipment_date DATE NOT NULL DEFAULT CURRENT_DATE,

    -- Трек-номер
    tracking_number VARCHAR(100),
    delivery_service VARCHAR(100),

    -- Статус
    status VARCHAR(30) DEFAULT 'CREATED' CHECK (status IN (
        'CREATED', 'CONFIRMED', 'IN_TRANSIT', 'DELIVERED', 'RETURNED', 'CANCELED'
    )),

    -- Данные для маркетплейса
    raw_data JSONB,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE marketplace_shipment IS 'Отгрузки на маркетплейсы';

-- 3.5. Складские остатки для маркетплейсов
CREATE TABLE marketplace_stock (
    id BIGSERIAL PRIMARY KEY,
    marketplace_settings_id BIGINT NOT NULL REFERENCES marketplace_settings(id),
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),

    -- Количество
    available_quantity INT NOT NULL DEFAULT 0,
    reserved_quantity INT DEFAULT 0,
    quantity_for_sync INT GENERATED ALWAYS AS (available_quantity - reserved_quantity) STORED,

    -- Внешние данные
    external_warehouse_id VARCHAR(50),
    last_sync_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (marketplace_settings_id, product_id, warehouse_id)
);

COMMENT ON TABLE marketplace_stock IS 'Остатки для маркетплейсов (облегченная версия для синхронизации)';

CREATE INDEX idx_mp_stock_marketplace ON marketplace_stock(marketplace_settings_id);
CREATE INDEX idx_mp_stock_product ON marketplace_stock(product_id);

-- 3.6. Цены для маркетплейсов
CREATE TABLE marketplace_price (
    id BIGSERIAL PRIMARY KEY,
    marketplace_settings_id BIGINT NOT NULL REFERENCES marketplace_settings(id),
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),

    price DECIMAL(15,2) NOT NULL,
    price_with_discount DECIMAL(15,2),

    -- Для маркетплейсов
    external_price_id VARCHAR(100),
    last_sync_at TIMESTAMP,

    valid_from DATE DEFAULT CURRENT_DATE,
    valid_to DATE,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE (marketplace_settings_id, product_id, valid_from)
);

COMMENT ON TABLE marketplace_price IS 'Цены для маркетплейсов';

-- 3.7. Логи синхронизации с маркетплейсами
CREATE TABLE marketplace_sync_log (
    id BIGSERIAL PRIMARY KEY,
    marketplace_settings_id BIGINT NOT NULL REFERENCES marketplace_settings(id),

    -- Тип синхронизации
    sync_type VARCHAR(30) NOT NULL CHECK (sync_type IN (
        'ORDERS', 'STOCKS', 'PRICES', 'PRODUCTS', 'SHIPMENTS', 'RETURNS'
    )),

    -- Статус
    status VARCHAR(20) DEFAULT 'STARTED' CHECK (status IN ('STARTED', 'PROCESSING', 'COMPLETED', 'FAILED')),

    -- Данные
    request_payload JSONB,
    response_payload JSONB,
    error_message TEXT,

    -- Количество обработанных записей
    processed_count INT DEFAULT 0,
    success_count INT DEFAULT 0,
    failed_count INT DEFAULT 0,

    started_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE marketplace_sync_log IS 'Лог синхронизации с маркетплейсами';

CREATE INDEX idx_mp_sync_marketplace ON marketplace_sync_log(marketplace_settings_id);
CREATE INDEX idx_mp_sync_status ON marketplace_sync_log(status);
CREATE INDEX idx_mp_sync_started ON marketplace_sync_log(started_at);

-- ============================================================
-- 4. УНИВЕРСАЛЬНЫЙ ЛОГ ИНТЕГРАЦИЙ
-- ============================================================

-- 4.1. Центральный лог всех интеграций
CREATE TABLE integration_log (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),

    -- Тип интеграции
    integration_type VARCHAR(30) NOT NULL CHECK (integration_type IN (
        'OFR', 'EGIAS', 'FNS', 'OZON', 'WILDBERRIES', 'YANDEX_MARKET', 'EDI', 'GIS_MT', 'OTHER'
    )),

    -- Направление
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('IN', 'OUT')),

    -- Метод/URL
    endpoint VARCHAR(255),
    method VARCHAR(20),

    -- Данные
    request_data JSONB,
    response_data JSONB,

    -- Статус
    status_code INT,
    status_message TEXT,
    is_error BOOLEAN DEFAULT FALSE,
    error_message TEXT,

    -- Время выполнения
    duration_ms INT,

    -- Идентификаторы
    external_id VARCHAR(100),
    document_number VARCHAR(50),
    document_id BIGINT,

    -- Кто инициировал
    initiated_by_id BIGINT REFERENCES employee(id),

    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE integration_log IS 'Центральный лог всех интеграций';

CREATE INDEX idx_int_log_type ON integration_log(integration_type);
CREATE INDEX idx_int_log_error ON integration_log(is_error) WHERE is_error = TRUE;
CREATE INDEX idx_int_log_created ON integration_log(created_at);
CREATE INDEX idx_int_log_doc ON integration_log(document_type, document_id);

-- ============================================================
-- 5. ИНДЕКСЫ ДЛЯ НОВЫХ ТАБЛИЦ (дополнительные)
-- ============================================================

CREATE INDEX idx_ofd_settings_org ON ofd_settings(organization_id);
CREATE INDEX idx_ofd_send_receipt ON ofd_send_status(receipt_id);
CREATE INDEX idx_ofd_send_status ON ofd_send_status(status);
CREATE INDEX idx_ofd_registration_cr ON ofd_registration(cash_register_id);

CREATE INDEX idx_alcohol_license_org ON alcohol_license(organization_id);
CREATE INDEX idx_alcohol_license_expiry ON alcohol_license(expiry_date) WHERE is_active = TRUE;
CREATE INDEX idx_alcohol_dec_org ON alcohol_declaration(organization_id);
CREATE INDEX idx_alcohol_dec_period ON alcohol_declaration(declaration_year, declaration_quarter);
CREATE INDEX idx_alcohol_dec_status ON alcohol_declaration(status);
CREATE INDEX idx_alcohol_dec_section ON alcohol_declaration_section(declaration_id);
CREATE INDEX idx_alcohol_stamp_code ON alcohol_excise_stamp(full_stamp_code);
CREATE INDEX idx_alcohol_stamp_status ON alcohol_excise_stamp(status);
CREATE INDEX idx_alcohol_stamp_marking ON alcohol_excise_stamp(marking_code_id);
CREATE INDEX idx_alcohol_journal_period ON alcohol_journal(journal_year, journal_month);
CREATE INDEX idx_alcohol_journal_product ON alcohol_journal(product_id);
CREATE INDEX idx_egais_settings_org ON egais_settings(organization_id);

CREATE INDEX idx_mp_settings_org ON marketplace_settings(organization_id);
CREATE INDEX idx_mp_settings_code ON marketplace_settings(marketplace_code);
CREATE INDEX idx_mp_order_settings ON marketplace_order(marketplace_settings_id);
CREATE INDEX idx_mp_order_status ON marketplace_order(status);
CREATE INDEX idx_mp_order_ext ON marketplace_order(external_order_id);
CREATE INDEX idx_mp_order_item_order ON marketplace_order_item(order_id);
CREATE INDEX idx_mp_order_item_product ON marketplace_order_item(product_id);
CREATE INDEX idx_mp_shipment_order ON marketplace_shipment(sales_order_id);
CREATE INDEX idx_mp_stock_marketplace ON marketplace_stock(marketplace_settings_id);
CREATE INDEX idx_mp_stock_product ON marketplace_stock(product_id);
CREATE INDEX idx_mp_price_marketplace ON marketplace_price(marketplace_settings_id);
CREATE INDEX idx_mp_price_product ON marketplace_price(product_id);
CREATE INDEX idx_mp_sync_marketplace ON marketplace_sync_log(marketplace_settings_id);
CREATE INDEX idx_mp_sync_status ON marketplace_sync_log(status);

CREATE INDEX idx_int_log_type ON integration_log(integration_type);
CREATE INDEX idx_int_log_error ON integration_log(is_error) WHERE is_error = TRUE;
CREATE INDEX idx_int_log_created ON integration_log(created_at);
CREATE INDEX idx_int_log_doc ON integration_log(document_type, document_id);

-- ============================================================
-- 6. ТРИГГЕРЫ ДЛЯ ОБНОВЛЕНИЯ UPDATED_AT
-- ============================================================

DO $$
DECLARE
    table_name text;
BEGIN
    FOR table_name IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename IN (
            'ofd_settings', 'alcohol_license', 'alcohol_declaration',
            'alcohol_excise_stamp', 'alcohol_journal', 'egais_settings',
            'marketplace_settings', 'marketplace_order', 'marketplace_shipment',
            'marketplace_stock', 'marketplace_price', 'marketplace_sync_log'
        )
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
-- 7. МАТЕРИАЛИЗОВАННОЕ ПРЕДСТАВЛЕНИЕ ДЛЯ СВОДКИ ПО МАРКЕТПЛЕЙСАМ
-- ============================================================

CREATE MATERIALIZED VIEW mv_marketplace_summary AS
SELECT
    mp_settings.marketplace_code,
    mp_settings.seller_id,
    COUNT(DISTINCT mp_order.id) AS total_orders,
    COUNT(DISTINCT mp_order.id) FILTER (WHERE mp_order.status IN ('NEW', 'ACCEPTED')) AS pending_orders,
    COALESCE(SUM(mp_order.total_amount), 0) AS total_sales_amount,
    AVG(mp_order.total_amount) AS avg_order_amount,
    COUNT(DISTINCT mp_shipment.id) AS total_shipments,
    COUNT(DISTINCT mp_shipment.id) FILTER (WHERE mp_shipment.status = 'DELIVERED') AS delivered_shipments,
    MAX(mp_sync_log.created_at) AS last_sync_at
FROM marketplace_settings mp_settings
LEFT JOIN marketplace_order mp_order ON mp_settings.id = mp_order.marketplace_settings_id
LEFT JOIN marketplace_shipment mp_shipment ON mp_settings.id = mp_shipment.marketplace_settings_id
LEFT JOIN marketplace_sync_log mp_sync_log ON mp_settings.id = mp_sync_log.marketplace_settings_id
GROUP BY mp_settings.marketplace_code, mp_settings.seller_id
WITH NO DATA;

CREATE UNIQUE INDEX idx_mv_marketplace_summary ON mv_marketplace_summary(marketplace_code, seller_id);

COMMENT ON MATERIALIZED VIEW mv_marketplace_summary IS 'Сводка по заказам и продажам на маркетплейсах';

-- ============================================================
-- 8. ФУНКЦИИ ДЛЯ РАБОТЫ С ИНТЕГРАЦИЯМИ
-- ============================================================

-- 8.1. Функция записи в интеграционный лог
CREATE OR REPLACE FUNCTION log_integration(
    p_organization_id BIGINT,
    p_integration_type VARCHAR,
    p_direction VARCHAR,
    p_endpoint VARCHAR,
    p_method VARCHAR,
    p_request_data JSONB,
    p_response_data JSONB,
    p_status_code INT,
    p_is_error BOOLEAN,
    p_error_message TEXT,
    p_duration_ms INT,
    p_external_id VARCHAR,
    p_document_type VARCHAR,
    p_document_id BIGINT,
    p_initiated_by_id BIGINT
) RETURNS BIGINT AS $$
DECLARE
    v_log_id BIGINT;
BEGIN
    INSERT INTO integration_log (
        organization_id,
        integration_type,
        direction,
        endpoint,
        method,
        request_data,
        response_data,
        status_code,
        is_error,
        error_message,
        duration_ms,
        external_id,
        document_type,
        document_id,
        initiated_by_id,
        created_at
    ) VALUES (
        p_organization_id,
        p_integration_type,
        p_direction,
        p_endpoint,
        p_method,
        p_request_data,
        p_response_data,
        p_status_code,
        COALESCE(p_is_error, p_status_code >= 400),
        p_error_message,
        p_duration_ms,
        p_external_id,
        p_document_type,
        p_document_id,
        p_initiated_by_id,
        NOW()
    ) RETURNING id INTO v_log_id;

    RETURN v_log_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION log_integration IS 'Универсальная функция для записи в лог интеграций';

-- 8.2. Функция обновления статуса отправки в ОФД
CREATE OR REPLACE FUNCTION update_ofd_status(
    p_receipt_id BIGINT,
    p_status VARCHAR,
    p_ofd_response_json JSONB,
    p_fiscal_document_number VARCHAR,
    p_fiscal_sign VARCHAR,
    p_fiscal_document_datetime TIMESTAMP,
    p_fns_site_url VARCHAR
) RETURNS VOID AS $$
BEGIN
    UPDATE ofd_send_status
    SET
        status = p_status,
        ofd_response_json = p_ofd_response_json,
        ofd_response_datetime = NOW(),
        fiscal_document_number = COALESCE(p_fiscal_document_number, fiscal_document_number),
        fiscal_sign = COALESCE(p_fiscal_sign, fiscal_sign),
        fiscal_document_datetime = COALESCE(p_fiscal_document_datetime, fiscal_document_datetime),
        fns_site_url = COALESCE(p_fns_site_url, fns_site_url),
        updated_at = NOW()
    WHERE receipt_id = p_receipt_id
    ORDER BY created_at DESC
    LIMIT 1;

    -- Обновляем основной чек
    UPDATE sales_receipt
    SET
        is_uploaded_to_fns = (p_status = 'COMPLETED'),
        uploaded_at = CASE WHEN p_status = 'COMPLETED' THEN NOW() ELSE uploaded_at END,
        fiscal_document_number = COALESCE(p_fiscal_document_number, fiscal_document_number),
        fiscal_sign = COALESCE(p_fiscal_sign, fiscal_sign),
        fiscal_document_date = COALESCE(p_fiscal_document_datetime, fiscal_document_date),
        fns_site_url = COALESCE(p_fns_site_url, fns_site_url),
        updated_at = NOW()
    WHERE id = p_receipt_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_ofd_status IS 'Обновление статуса отправки чека в ОФД';

-- 8.3. Функция создания заказа с маркетплейса
CREATE OR REPLACE FUNCTION create_marketplace_order(
    p_organization_id BIGINT,
    p_marketplace_settings_id BIGINT,
    p_external_order_id VARCHAR,
    p_buyer_name VARCHAR,
    p_buyer_phone VARCHAR,
    p_buyer_address TEXT,
    p_total_amount DECIMAL,
    p_delivery_amount DECIMAL,
    p_marketplace_status VARCHAR,
    p_raw_data JSONB,
    p_items JSONB -- [{product_id, quantity, price, external_product_id}]
) RETURNS BIGINT AS $$
DECLARE
    v_order_id BIGINT;
    v_item JSONB;
BEGIN
    -- Создаем заказ
    INSERT INTO marketplace_order (
        organization_id,
        marketplace_settings_id,
        external_order_id,
        order_number,
        buyer_name,
        buyer_phone,
        buyer_address,
        total_amount,
        delivery_amount,
        marketplace_status,
        raw_data
    ) VALUES (
        p_organization_id,
        p_marketplace_settings_id,
        p_external_order_id,
        'MP-' || p_external_order_id,
        p_buyer_name,
        p_buyer_phone,
        p_buyer_address,
        p_total_amount,
        COALESCE(p_delivery_amount, 0),
        p_marketplace_status,
        p_raw_data
    ) RETURNING id INTO v_order_id;

    -- Создаем строки заказа
    FOR v_item IN SELECT * FROM jsonb_array_elements(p_items)
    LOOP
        INSERT INTO marketplace_order_item (
            order_id,
            product_id,
            external_product_id,
            product_name,
            product_sku,
            quantity,
            price
        ) VALUES (
            v_order_id,
            (v_item->>'product_id')::BIGINT,
            v_item->>'external_product_id',
            v_item->>'product_name',
            v_item->>'product_sku',
            (v_item->>'quantity')::INT,
            (v_item->>'price')::DECIMAL
        );
    END LOOP;

    RETURN v_order_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION create_marketplace_order IS 'Создание заказа с маркетплейса';
