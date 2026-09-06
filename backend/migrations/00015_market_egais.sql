-- Этап 15: маркетплейсы и ЕГАИС (по sql/02). Отличия: timestamptz,
-- привязка к provider_code реестра (без marketplace_settings — ключи в
-- integration_settings), без plpgsql. Алкогольные декларации/акцизы — вне скоупа.

-- Связка товара с оффером маркетплейса.
CREATE TABLE IF NOT EXISTS market_offer_link (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    provider_code VARCHAR(50) NOT NULL,
    product_id BIGINT NOT NULL REFERENCES catalog_product(id) ON DELETE CASCADE,
    offer_id VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, provider_code, offer_id)
);

-- Заказы с маркетплейсов.
CREATE TABLE IF NOT EXISTS marketplace_order (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    provider_code VARCHAR(50) NOT NULL,
    external_order_id VARCHAR(100) NOT NULL,
    external_created_at TIMESTAMPTZ,
    buyer_name VARCHAR(255),
    buyer_phone VARCHAR(20),
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    marketplace_status VARCHAR(50),
    status VARCHAR(30) DEFAULT 'NEW' CHECK (status IN (
        'NEW', 'MATCHED', 'SKIPPED', 'CANCELED'
    )),
    sales_order_id BIGINT REFERENCES sales_order(id) ON DELETE SET NULL,
    raw_data JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, provider_code, external_order_id)
);
CREATE INDEX IF NOT EXISTS idx_mp_order_org ON marketplace_order(organization_id);

-- Позиции заказов маркетплейсов.
CREATE TABLE IF NOT EXISTS marketplace_order_item (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES marketplace_order(id) ON DELETE CASCADE,
    product_id BIGINT REFERENCES catalog_product(id) ON DELETE SET NULL,
    external_offer_id VARCHAR(100),
    product_name VARCHAR(255) NOT NULL,
    quantity DECIMAL(15,3) NOT NULL,
    price DECIMAL(15,2) NOT NULL
);

-- Журнал синхронизаций.
CREATE TABLE IF NOT EXISTS market_sync_log (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    provider_code VARCHAR(50) NOT NULL,
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('IN','OUT')),
    operation VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('OK','PARTIAL','FAILED')),
    items_total INT DEFAULT 0,
    items_ok INT DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sync_log_org ON market_sync_log(organization_id);

-- Документы ЕГАИС (outbox в УТМ).
CREATE TABLE IF NOT EXISTS egais_document (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    doc_type VARCHAR(50) NOT NULL,
    body_xml TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING','SENT','ACCEPTED','FAILED'
    )),
    reply TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_egais_doc_org ON egais_document(organization_id);
