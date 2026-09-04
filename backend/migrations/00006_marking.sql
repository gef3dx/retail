-- Этап 5: маркировка «Честный знак» (по sql/01 marking_* + sql/02 integration_log).
-- Отложено: supplier/counterparty FK, receipt_document/shipment FK, алкоголь (этапы 6/9).

-- Пул кодов маркировки (DataMatrix). Один код — одна единица товара.
CREATE TABLE IF NOT EXISTS marking_code_pool (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE RESTRICT,
    code VARCHAR(150) UNIQUE NOT NULL,
    gtin VARCHAR(14) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE'
        CHECK (status IN ('AVAILABLE','SOLD','RETURNED','WRITTEN_OFF')),
    batch_id BIGINT,
    sales_receipt_id BIGINT REFERENCES sales_receipt(id) ON DELETE SET NULL,
    received_at TIMESTAMPTZ DEFAULT NOW(),
    sold_at TIMESTAMPTZ,
    expiry_date DATE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_marking_product ON marking_code_pool(product_id);
CREATE INDEX IF NOT EXISTS idx_marking_status ON marking_code_pool(status);
CREATE INDEX IF NOT EXISTS idx_marking_gtin ON marking_code_pool(gtin);

-- Пачки кодов (загрузка из файла/ЭДО одним батчем).
CREATE TABLE IF NOT EXISTS marking_batch (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE RESTRICT,
    batch_number VARCHAR(50) UNIQUE NOT NULL,
    total_codes INT NOT NULL CHECK (total_codes > 0),
    used_codes INT DEFAULT 0,
    status VARCHAR(20) DEFAULT 'CONFIRMED'
        CHECK (status IN ('CREATED','CONFIRMED','CLOSED')),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
ALTER TABLE marking_code_pool
    ADD CONSTRAINT fk_marking_batch FOREIGN KEY (batch_id) REFERENCES marking_batch(id) ON DELETE SET NULL;

-- Связь чек ↔ коды (sql/01 receipt_marking_link, cashier → users).
CREATE TABLE IF NOT EXISTS receipt_marking_link (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES sales_receipt(id) ON DELETE CASCADE,
    marking_code_id BIGINT NOT NULL UNIQUE REFERENCES marking_code_pool(id) ON DELETE RESTRICT,
    cashier_id BIGINT REFERENCES users(id),
    scanned_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_rm_link_receipt ON receipt_marking_link(receipt_id);

-- Настройки ГИС МТ (mock по умолчанию; настоящий API — этап 9).
CREATE TABLE IF NOT EXISTS gismt_settings (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    provider VARCHAR(30) NOT NULL DEFAULT 'MOCK',
    auto_send_enabled BOOLEAN DEFAULT TRUE,
    max_retries INT DEFAULT 5,
    fail_first_attempts INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    UNIQUE (organization_id, provider)
);

-- Очередь операций в ГИС МТ: WITHDRAW (продажа), RETURN (возврат), WRITE_OFF, RECEIVE (приёмка).
CREATE TABLE IF NOT EXISTS gismt_queue (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    marking_code_id BIGINT NOT NULL REFERENCES marking_code_pool(id) ON DELETE CASCADE,
    operation VARCHAR(20) NOT NULL CHECK (operation IN ('WITHDRAW','RETURN','WRITE_OFF','RECEIVE')),
    receipt_id BIGINT REFERENCES sales_receipt(id) ON DELETE SET NULL,
    send_attempt INT DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    status VARCHAR(20) DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','RETRY','COMPLETED','FAILED')),
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_gismt_pending ON gismt_queue(status) WHERE status IN ('PENDING','RETRY');
CREATE INDEX IF NOT EXISTS idx_gismt_code ON gismt_queue(marking_code_id);

-- Центральный лог интеграций (sql/02, initiated_by → users вместо employee).
CREATE TABLE IF NOT EXISTS integration_log (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    integration_type VARCHAR(30) NOT NULL CHECK (integration_type IN (
        'OFD','EGAIS','FNS','OZON','WILDBERRIES','YANDEX_MARKET','EDI','GIS_MT','OTHER')),
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('IN','OUT')),
    endpoint VARCHAR(255),
    request_data JSONB,
    response_data JSONB,
    is_error BOOLEAN DEFAULT FALSE,
    error_message TEXT,
    external_id VARCHAR(100),
    document_id BIGINT,
    initiated_by_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_int_log_type ON integration_log(integration_type);
CREATE INDEX IF NOT EXISTS idx_int_log_created ON integration_log(created_at);
