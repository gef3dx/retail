-- Этап 16: налоговый учёт (по sql/01: книги покупок/продаж + закрытия периодов).
-- Книги — снапшоты закрытых кварталов; живые данные всегда доступны
-- вычислением из документов (см. репозиторий). Регистры УСН/прибыли — вне скоупа v1.

CREATE TABLE IF NOT EXISTS purchase_book (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    year INT NOT NULL,
    quarter INT NOT NULL CHECK (quarter BETWEEN 1 AND 4),
    entry_number INT NOT NULL,
    document_type VARCHAR(50) NOT NULL,
    document_number VARCHAR(50),
    document_date DATE,
    supplier_inn VARCHAR(12),
    purchase_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    vat_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    vat_rate DECIMAL(5,2),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, year, quarter, entry_number)
);

CREATE TABLE IF NOT EXISTS sales_book (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    year INT NOT NULL,
    quarter INT NOT NULL CHECK (quarter BETWEEN 1 AND 4),
    entry_number INT NOT NULL,
    document_type VARCHAR(50) NOT NULL,
    document_number VARCHAR(50),
    document_date DATE,
    sales_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    vat_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_amount DECIMAL(15,2) NOT NULL DEFAULT 0,
    vat_rate DECIMAL(5,2),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, year, quarter, entry_number)
);

-- Декларации (сводка закрытого периода).
CREATE TABLE IF NOT EXISTS tax_declaration (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    year INT NOT NULL,
    quarter INT NOT NULL CHECK (quarter BETWEEN 1 AND 4),
    decl_type VARCHAR(20) NOT NULL DEFAULT 'NDS' CHECK (decl_type IN ('NDS','USN','PROFIT')),
    total_sales DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat_out DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_purchases DECIMAL(15,2) NOT NULL DEFAULT 0,
    total_vat_in DECIMAL(15,2) NOT NULL DEFAULT 0,
    vat_due DECIMAL(15,2) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','SUBMITTED')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, year, quarter, decl_type)
);
