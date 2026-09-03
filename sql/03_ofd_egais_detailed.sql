-- ============================================================
-- 03_ofd_egais_detailed.sql – Расширенный учет ОФД (54-ФЗ) и ЕГАИС
-- ============================================================

-- ============================================================
-- 1. ОФД - РАСШИРЕННЫЕ ЧЕКИ ПО 54-ФЗ
-- ============================================================

-- 1.1. Типы чеков по 54-ФЗ (справочник)
CREATE TABLE ffd_receipt_type (
    code VARCHAR(10) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE
);

COMMENT ON TABLE ffd_receipt_type IS 'Типы чеков по 54-ФЗ (ФФД 1.0, 1.05, 1.1, 1.2)';

INSERT INTO ffd_receipt_type (code, name, description) VALUES
('FFD_1.0', 'ФФД 1.0', 'Формат фискальных данных версии 1.0'),
('FFD_1.05', 'ФФД 1.05', 'Формат фискальных данных версии 1.05'),
('FFD_1.1', 'ФФД 1.1', 'Формат фискальных данных версии 1.1'),
('FFD_1.2', 'ФФД 1.2', 'Формат фискальных данных версии 1.2 (актуальный)');

-- 1.2. Признаки предмета расчета (справочник)
CREATE TABLE ffd_item_attribute (
    code INT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT
);

COMMENT ON TABLE ffd_item_attribute IS 'Признаки предмета расчета (тег 1212)';

INSERT INTO ffd_item_attribute (code, name, description) VALUES
(1, 'Товар', 'Товар (за исключением подакцизного товара, сырья, товаров, подлежащих маркировке)'),
(2, 'Подакцизный товар', 'Подакцизный товар (за исключением товаров, подлежащих маркировке)'),
(3, 'Работа', 'Работа'),
(4, 'Услуга', 'Услуга'),
(5, 'Ставка в азартной игре', 'Ставка в азартной игре'),
(6, 'Выигрыш в азартной игре', 'Выигрыш в азартной игре'),
(7, 'Лотерейный билет', 'Лотерейный билет'),
(8, 'Выигрыш в лотерее', 'Выигрыш в лотерее'),
(9, 'Предоставление прав на РИД', 'Предоставление прав на РИД'),
(10, 'Аванс/предоплата', 'Аванс, задаток, предоплата, кредит'),
(11, 'Платеж', 'Платеж, частичная оплата предмета расчета'),
(12, 'Прием', 'Прием денежных средств по договору займа'),
(13, 'Выплата', 'Выплата денежных средств по договору займа'),
(14, 'Иной предмет расчета', 'Иной предмет расчета');

-- 1.3. Признаки способа расчета (справочник)
CREATE TABLE ffd_payment_method (
    code INT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT
);

COMMENT ON TABLE ffd_payment_method IS 'Признаки способа расчета (тег 1214)';

INSERT INTO ffd_payment_method (code, name, description) VALUES
(1, 'Полная предоплата', 'Полная предоплата до момента передачи предмета расчета'),
(2, 'Частичная предоплата', 'Частичная предоплата до момента передачи предмета расчета'),
(3, 'Аванс', 'Аванс'),
(4, 'Полный расчет', 'Полный расчет в момент передачи предмета расчета'),
(5, 'Частичный расчет', 'Частичный расчет в момент передачи предмета расчета с последующей оплатой в кредит'),
(6, 'Передача в кредит', 'Передача предмета расчета без его оплаты в момент передачи с последующей оплатой в кредит'),
(7, 'Оплата кредита', 'Оплата предмета расчета после его передачи с оплатой в кредит');

-- 1.4. Расширенная информация о чеке (ФФД)
CREATE TABLE receipt_ffd_data (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES sales_receipt(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    
    -- Версия ФФД
    ffd_version VARCHAR(10) NOT NULL DEFAULT 'FFD_1.2' REFERENCES ffd_receipt_type(code),
    
    -- Фискальные признаки (из ФН)
    fiscal_attribute VARCHAR(30),
    fiscal_document_number VARCHAR(20),
    fiscal_document_datetime TIMESTAMP,
    fiscal_drive_number VARCHAR(20),
    fiscal_sign VARCHAR(30),
    fiscal_shift_number INT,
    fiscal_receipt_number INT,
    
    -- Данные ККТ
    kkt_reg_number VARCHAR(20),
    kkt_factory_number VARCHAR(30),
    kkt_model VARCHAR(50),
    
    -- Данные ОФД
    ofd_name VARCHAR(100),
    ofd_inn VARCHAR(12),
    ofd_receipt_url VARCHAR(255),
    ofd_response_code VARCHAR(10),
    
    -- Данные ФНС
    fns_site_url VARCHAR(255),
    fns_qr_code_url VARCHAR(255),
    
    -- Теги ФФД
    tags_json JSONB,
    
    -- Статус
    is_registered BOOLEAN DEFAULT FALSE,
    registered_at TIMESTAMP,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE receipt_ffd_data IS 'Расширенные фискальные данные чека (ФФД)';

CREATE INDEX idx_ffd_receipt ON receipt_ffd_data(receipt_id);
CREATE INDEX idx_ffd_fiscal_sign ON receipt_ffd_data(fiscal_sign);

-- 1.5. Детальные позиции чека (с расшифровкой по ФФД)
CREATE TABLE receipt_ffd_item (
    id BIGSERIAL PRIMARY KEY,
    receipt_ffd_id BIGINT NOT NULL REFERENCES receipt_ffd_data(id) ON DELETE CASCADE,
    
    -- Связь с товаром в чеке
    receipt_item_id BIGINT REFERENCES sales_receipt_item(id),
    product_id BIGINT REFERENCES catalog_product(id),
    
    -- Наименование (как в чеке)
    item_name VARCHAR(255) NOT NULL,
    item_code VARCHAR(50),
    
    -- Количество и цена
    quantity DECIMAL(15,3) NOT NULL,
    price DECIMAL(15,2) NOT NULL,
    total_amount DECIMAL(15,2) NOT NULL,
    
    -- Ставка НДС (код по ФФД)
    vat_code INT NOT NULL CHECK (vat_code IN (
        1,  -- 20%
        2,  -- 10%
        3,  -- 20/120
        4,  -- 10/110
        5,  -- 0%
        6,  -- Без НДС
        7   -- НДС не облагается
    )),
    vat_rate DECIMAL(5,2),
    vat_amount DECIMAL(15,2),
    
    -- Признаки по ФФД
    item_attribute_code INT REFERENCES ffd_item_attribute(code),
    payment_method_code INT REFERENCES ffd_payment_method(code),
    payment_subject_code INT,
    
    -- Для маркировки
    is_marked BOOLEAN DEFAULT FALSE,
    marking_code VARCHAR(150),
    
    -- Для алкоголя
    is_alcohol BOOLEAN DEFAULT FALSE,
    alcohol_volume DECIMAL(5,2),
    alcohol_type VARCHAR(50),
    
    -- Скидки
    discount_amount DECIMAL(15,2) DEFAULT 0,
    discount_description VARCHAR(255),
    
    -- Дополнительные теги (JSON)
    extra_tags JSONB,
    
    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE receipt_ffd_item IS 'Детальные позиции чека с расшифровкой по ФФД';

CREATE INDEX idx_ffd_item_receipt ON receipt_ffd_item(receipt_ffd_id);
CREATE INDEX idx_ffd_item_product ON receipt_ffd_item(product_id);
CREATE INDEX idx_ffd_item_marking ON receipt_ffd_item(marking_code) WHERE marking_code IS NOT NULL;

-- 1.6. Признаки платежа (справочник)
CREATE TABLE ffd_payment_sign (
    code INT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT
);

INSERT INTO ffd_payment_sign (code, name, description) VALUES
(1, 'Наличные', 'Оплата наличными деньгами'),
(2, 'Безналичные', 'Оплата банковской картой/электронными средствами'),
(3, 'Предоплата', 'Предоплата (зачет аванса)'),
(4, 'Постоплата', 'Постоплата (кредит)'),
(5, 'Иная форма', 'Иная форма оплаты');

-- 1.7. Транзакции оплаты в чеке
CREATE TABLE receipt_payment_transaction (
    id BIGSERIAL PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES sales_receipt(id) ON DELETE CASCADE,
    
    -- Тип платежа
    payment_type VARCHAR(20) NOT NULL CHECK (payment_type IN ('CASH', 'CARD', 'PREPAYMENT', 'CREDIT', 'OTHER')),
    payment_sign INT REFERENCES ffd_payment_sign(code),
    
    -- Сумма
    amount DECIMAL(15,2) NOT NULL,
    
    -- Данные карты/транзакции (для безналичных)
    card_pan_mask VARCHAR(19),
    card_holder_name VARCHAR(100),
    transaction_id VARCHAR(50),
    bank_name VARCHAR(100),
    bank_inn VARCHAR(12),
    
    -- Данные электронных средств
    ewallet_id VARCHAR(50),
    ewallet_operator VARCHAR(50),
    
    -- Данные предоплаты
    prepayment_document_number VARCHAR(50),
    prepayment_document_date DATE,
    
    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE receipt_payment_transaction IS 'Транзакции оплаты в чеке';

CREATE INDEX idx_payment_receipt ON receipt_payment_transaction(receipt_id);

-- 1.8. Коррекционные чеки (54-ФЗ)
CREATE TABLE receipt_correction (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    cash_register_id BIGINT NOT NULL REFERENCES cash_register(id),
    
    -- Данные коррекции
    correction_number VARCHAR(20) NOT NULL,
    correction_date DATE NOT NULL,
    correction_type VARCHAR(20) CHECK (correction_type IN ('SELF', 'FNS_ORDER')),
    
    -- Причина коррекции
    correction_reason TEXT,
    correction_order_number VARCHAR(50),
    correction_order_date DATE,
    
    -- Суммы
    total_amount DECIMAL(15,2) NOT NULL,
    total_vat DECIMAL(15,2) DEFAULT 0,
    
    -- Фискальные данные
    fiscal_document_number VARCHAR(20),
    fiscal_sign VARCHAR(30),
    
    -- Статус
    status VARCHAR(20) DEFAULT 'CREATED' CHECK (status IN ('CREATED', 'SENT', 'REGISTERED', 'FAILED')),
    
    -- Данные ФФД
    ffd_data JSONB,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE receipt_correction IS 'Коррекционные чеки';

CREATE INDEX idx_correction_org ON receipt_correction(organization_id);
CREATE INDEX idx_correction_cr ON receipt_correction(cash_register_id);

-- ============================================================
-- 2. ЕГАИС - РАСШИРЕННЫЙ УЧЕТ АЛКОГОЛЯ
-- ============================================================

-- 2.1. Справочник видов алкогольной продукции
CREATE TABLE alcohol_product_type (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    alcohol_min DECIMAL(5,2),
    alcohol_max DECIMAL(5,2),
    measurement_unit VARCHAR(10) DEFAULT 'дал',
    is_active BOOLEAN DEFAULT TRUE
);

COMMENT ON TABLE alcohol_product_type IS 'Виды алкогольной продукции (по классификации ЕГАИС)';

INSERT INTO alcohol_product_type (code, name, description, alcohol_min, alcohol_max) VALUES
('VODKA', 'Водка', 'Водка и ликероводочные изделия крепостью более 38%', 38, 56),
('LIQUEUR', 'Ликер', 'Ликеры и десертные напитки', 15, 45),
('COGNAC', 'Коньяк', 'Коньяк и бренди', 38, 45),
('WINE', 'Вино', 'Вина столовые', 8, 16),
('CHAMPAGNE', 'Шампанское', 'Игристые вина', 9, 14),
('BEER', 'Пиво', 'Пиво и пивные напитки', 0.5, 12),
('CIDER', 'Сидр', 'Сидр и медовуха', 3, 8),
('STRONG_ALCOHOL', 'Крепкий алкоголь', 'Крепкие спиртные напитки', 38, 70);

-- 2.2. Детальная информация о товаре (алкоголь)
CREATE TABLE alcohol_product_detail (
    product_id BIGINT PRIMARY KEY REFERENCES catalog_product(id),
    
    -- Тип алкоголя
    alcohol_type_code VARCHAR(20) NOT NULL REFERENCES alcohol_product_type(code),
    
    -- Крепость
    alcohol_volume DECIMAL(5,2) NOT NULL,
    alcohol_volume_min DECIMAL(5,2),
    alcohol_volume_max DECIMAL(5,2),
    
    -- Объем в бутылке
    bottle_volume DECIMAL(10,3) NOT NULL,
    bottle_volume_code VARCHAR(20),
    
    -- Код продукции по ЕГАИС
    egais_product_code VARCHAR(20),
    egais_product_group VARCHAR(50),
    
    -- Коды ТН ВЭД
    tnved_alcohol_code VARCHAR(20),
    
    -- Акциз
    excise_rate DECIMAL(15,2) NOT NULL,
    excise_rate_code VARCHAR(20),
    
    -- Данные о производителе (для ЕГАИС)
    producer_name VARCHAR(255),
    producer_inn VARCHAR(12),
    producer_address TEXT,
    producer_license_number VARCHAR(50),
    
    -- Для импортного алкоголя
    is_imported BOOLEAN DEFAULT FALSE,
    import_country VARCHAR(50),
    import_certificate_number VARCHAR(50),
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE alcohol_product_detail IS 'Детальная информация об алкогольной продукции';

-- 2.3. Акцизные марки (детальный учет)
CREATE TABLE alcohol_excise_stamp_detail (
    id BIGSERIAL PRIMARY KEY,
    excise_stamp_id BIGINT NOT NULL REFERENCES alcohol_excise_stamp(id) ON DELETE CASCADE,
    
    -- Полные данные марки
    stamp_series VARCHAR(10) NOT NULL,
    stamp_number VARCHAR(20) NOT NULL,
    stamp_full_code VARCHAR(50) NOT NULL,
    
    -- Данные по ЕГАИС
    egais_stamp_id VARCHAR(50),
    egais_stamp_barcode VARCHAR(50),
    
    -- Привязка к коду маркировки
    marking_code_id BIGINT REFERENCES marking_code_pool(id),
    
    -- Статус в ЕГАИС
    egais_status VARCHAR(30) DEFAULT 'REGISTERED' CHECK (egais_status IN (
        'REGISTERED', 'STICKED', 'SOLD', 'RETURNED', 'DESTROYED', 'LOST'
    )),
    
    -- Кто наклеил
    sticked_by_id BIGINT REFERENCES employee(id),
    sticked_at TIMESTAMP,
    
    -- Кто списал
    written_off_by_id BIGINT REFERENCES employee(id),
    written_off_at TIMESTAMP,
    written_off_reason TEXT,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE alcohol_excise_stamp_detail IS 'Детальный учет акцизных марок';

CREATE INDEX idx_stamp_detail_full ON alcohol_excise_stamp_detail(stamp_full_code);
CREATE INDEX idx_stamp_detail_egais ON alcohol_excise_stamp_detail(egais_stamp_id);

-- 2.4. Маршрутные листы ЕГАИС
CREATE TABLE egais_waybill (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    
    -- Номер маршрутного листа
    waybill_number VARCHAR(50) UNIQUE NOT NULL,
    waybill_date DATE NOT NULL,
    
    -- Тип
    waybill_type VARCHAR(30) CHECK (waybill_type IN (
        'INCOMING', 'OUTGOING', 'INTERNAL', 'RETURN'
    )),
    
    -- Контрагент
    counterparty_id BIGINT REFERENCES counterparty(id),
    counterparty_inn VARCHAR(12),
    counterparty_kpp VARCHAR(9),
    counterparty_name VARCHAR(255),
    
    -- Склады
    from_warehouse_id BIGINT REFERENCES warehouse(id),
    to_warehouse_id BIGINT REFERENCES warehouse(id),
    
    -- Данные по ЕГАИС
    egais_document_id VARCHAR(50),
    egais_document_number VARCHAR(50),
    egais_document_date DATE,
    egais_status VARCHAR(30) DEFAULT 'CREATED' CHECK (egais_status IN (
        'CREATED', 'SENT', 'RECEIVED', 'CONFIRMED', 'REJECTED', 'COMPLETED'
    )),
    
    -- Транспорт
    vehicle_number VARCHAR(20),
    driver_name VARCHAR(100),
    driver_license VARCHAR(20),
    
    -- Сопроводительные документы
    accompanying_documents JSONB,
    
    -- Суммы
    total_volume DECIMAL(15,3),
    total_amount DECIMAL(15,2),
    total_excise DECIMAL(15,2),
    
    -- XML данные (для ЕГАИС)
    xml_request_data BYTEA,
    xml_response_data BYTEA,
    
    -- Статус
    status VARCHAR(20) DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'CREATED', 'POSTED', 'CANCELED')),
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE egais_waybill IS 'Маршрутные листы для ЕГАИС';

CREATE INDEX idx_waybill_org ON egais_waybill(organization_id);
CREATE INDEX idx_waybill_number ON egais_waybill(waybill_number);
CREATE INDEX idx_waybill_egais ON egais_waybill(egais_document_id);
CREATE INDEX idx_waybill_type ON egais_waybill(waybill_type);
CREATE INDEX idx_waybill_date ON egais_waybill(waybill_date);

-- 2.5. Позиции маршрутного листа
CREATE TABLE egais_waybill_item (
    id BIGSERIAL PRIMARY KEY,
    waybill_id BIGINT NOT NULL REFERENCES egais_waybill(id) ON DELETE CASCADE,
    
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    product_name VARCHAR(255) NOT NULL,
    
    -- Количество
    quantity DECIMAL(15,3) NOT NULL,
    quantity_in_dal DECIMAL(15,3),
    quantity_in_bottles INT,
    
    -- Характеристики
    alcohol_volume DECIMAL(5,2),
    bottle_volume DECIMAL(10,3),
    
    -- Акциз
    excise_rate DECIMAL(15,2),
    excise_amount DECIMAL(15,2),
    
    -- Акцизные марки (массив)
    excise_stamp_ids BIGINT[],
    
    -- Цена и сумма
    price DECIMAL(15,2),
    total_amount DECIMAL(15,2),
    
    -- Данные по ЕГАИС
    egais_item_id VARCHAR(50),
    egais_batch_number VARCHAR(50),
    
    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE egais_waybill_item IS 'Позиции маршрутных листов ЕГАИС';

CREATE INDEX idx_waybill_item_waybill ON egais_waybill_item(waybill_id);
CREATE INDEX idx_waybill_item_product ON egais_waybill_item(product_id);

-- 2.6. Акт списания алкоголя (потери, брак)
CREATE TABLE egais_write_off_act (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    
    -- Номер акта
    act_number VARCHAR(50) UNIQUE NOT NULL,
    act_date DATE NOT NULL,
    
    -- Тип списания
    write_off_type VARCHAR(30) CHECK (write_off_type IN (
        'LOSS', 'DEFECT', 'SAMPLE', 'RETURN_SUPPLIER', 'DESTRUCTION'
    )),
    
    -- Комиссия
    commission_chairman_id BIGINT REFERENCES employee(id),
    commission_members BIGINT[],
    
    -- Данные ЕГАИС
    egais_document_id VARCHAR(50),
    egais_status VARCHAR(30) DEFAULT 'CREATED',
    
    -- Суммы
    total_volume DECIMAL(15,3),
    total_amount DECIMAL(15,2),
    total_excise DECIMAL(15,2),
    
    -- Причина
    reason TEXT,
    
    -- Статус
    status VARCHAR(20) DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED', 'CANCELED', 'APPROVED')),
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE egais_write_off_act IS 'Акты списания алкогольной продукции';

CREATE INDEX idx_writeoff_org ON egais_write_off_act(organization_id);
CREATE INDEX idx_writeoff_warehouse ON egais_write_off_act(warehouse_id);
CREATE INDEX idx_writeoff_egais ON egais_write_off_act(egais_document_id);

-- 2.7. Позиции акта списания
CREATE TABLE egais_write_off_item (
    id BIGSERIAL PRIMARY KEY,
    write_off_act_id BIGINT NOT NULL REFERENCES egais_write_off_act(id) ON DELETE CASCADE,
    
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    product_name VARCHAR(255) NOT NULL,
    
    quantity DECIMAL(15,3) NOT NULL,
    quantity_in_dal DECIMAL(15,3),
    
    -- Причина списания конкретной позиции
    reason TEXT,
    
    -- Акцизные марки (массив)
    excise_stamp_ids BIGINT[],
    
    -- Данные ЕГАИС
    egais_item_id VARCHAR(50),
    
    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE egais_write_off_item IS 'Позиции актов списания алкоголя';

-- 2.8. Остатки алкоголя в разрезе ЕГАИС
CREATE TABLE egais_balance (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouse(id),
    product_id BIGINT NOT NULL REFERENCES catalog_product(id),
    
    -- Остатки
    quantity DECIMAL(15,3) NOT NULL DEFAULT 0,
    quantity_in_dal DECIMAL(15,3) GENERATED ALWAYS AS (quantity / 10) STORED,
    
    -- В разрезе партий
    batch_number VARCHAR(50),
    production_date DATE,
    expiry_date DATE,
    
    -- Акцизные марки
    excise_stamp_ids BIGINT[],
    
    -- Данные ЕГАИС
    egais_batch_id VARCHAR(50),
    last_sync_at TIMESTAMP,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE (organization_id, warehouse_id, product_id, batch_number, production_date)
);

COMMENT ON TABLE egais_balance IS 'Остатки алкоголя для ЕГАИС';

CREATE INDEX idx_egais_balance_org ON egais_balance(organization_id);
CREATE INDEX idx_egais_balance_warehouse ON egais_balance(warehouse_id);
CREATE INDEX idx_egais_balance_product ON egais_balance(product_id);
CREATE INDEX idx_egais_balance_batch ON egais_balance(batch_number);

-- 2.9. Очередь сообщений ЕГАИС
CREATE TABLE egais_message_queue (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    
    -- Тип сообщения
    message_type VARCHAR(50) NOT NULL CHECK (message_type IN (
        'WAYBILL_IN', 'WAYBILL_OUT', 'WRITE_OFF', 'BALANCE_QUERY', 'STAMP_QUERY', 'REGISTRATION', 'ACKNOWLEDGE'
    )),
    
    -- Данные запроса (XML)
    request_xml TEXT,
    request_file_name VARCHAR(255),
    
    -- Данные ответа
    response_xml TEXT,
    response_file_name VARCHAR(255),
    response_code VARCHAR(20),
    response_message TEXT,
    
    -- Статус
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SENT', 'RECEIVED', 'PROCESSED', 'ERROR')),
    
    -- Попытки
    attempt_count INT DEFAULT 0,
    max_attempts INT DEFAULT 3,
    
    -- Связь с документом
    document_type VARCHAR(50),
    document_id BIGINT,
    
    created_at TIMESTAMP DEFAULT NOW(),
    processed_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE egais_message_queue IS 'Очередь сообщений для ЕГАИС';

CREATE INDEX idx_egais_queue_status ON egais_message_queue(status) WHERE status = 'PENDING';
CREATE INDEX idx_egais_queue_doc ON egais_message_queue(document_type, document_id);

-- 2.10. Журнал ЕГАИС (форма 11)
CREATE TABLE egais_journal (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organization(id),
    
    -- Период
    journal_year INT NOT NULL,
    journal_month INT NOT NULL CHECK (journal_month BETWEEN 1 AND 12),
    
    -- Номер записи
    entry_number INT NOT NULL,
    
    -- Дата
    entry_date DATE NOT NULL,
    
    -- Тип операции (по ЕГАИС)
    operation_code VARCHAR(20) NOT NULL,
    operation_name VARCHAR(100),
    
    -- Данные о продукции
    product_id BIGINT REFERENCES catalog_product(id),
    product_name VARCHAR(255),
    
    -- Количество
    quantity DECIMAL(15,3) NOT NULL,
    quantity_in_dal DECIMAL(15,3),
    
    -- Акцизные марки
    excise_stamp_ids BIGINT[],
    
    -- Документ-основание
    base_document_type VARCHAR(50),
    base_document_number VARCHAR(50),
    base_document_date DATE,
    
    -- Контрагент
    counterparty_inn VARCHAR(12),
    counterparty_name VARCHAR(255),
    
    -- Данные ЕГАИС
    egais_document_id VARCHAR(50),
    egais_transaction_id VARCHAR(50),
    
    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE egais_journal IS 'Журнал ЕГАИС (форма 11)';

CREATE INDEX idx_egais_journal_period ON egais_journal(journal_year, journal_month);
CREATE INDEX idx_egais_journal_product ON egais_journal(product_id);
CREATE INDEX idx_egais_journal_operation ON egais_journal(operation_code);

-- ============================================================
-- 3. ИНДЕКСЫ ДЛЯ НОВЫХ ТАБЛИЦ
-- ============================================================

CREATE INDEX idx_ffd_data_receipt ON receipt_ffd_data(receipt_id);
CREATE INDEX idx_ffd_data_fiscal ON receipt_ffd_data(fiscal_sign, fiscal_document_number);
CREATE INDEX idx_ffd_item_ffd ON receipt_ffd_item(receipt_ffd_id);
CREATE INDEX idx_ffd_item_product ON receipt_ffd_item(product_id);
CREATE INDEX idx_ffd_item_marking ON receipt_ffd_item(marking_code) WHERE marking_code IS NOT NULL;
CREATE INDEX idx_payment_transaction ON receipt_payment_transaction(receipt_id);
CREATE INDEX idx_correction_org ON receipt_correction(organization_id);
CREATE INDEX idx_correction_cr ON receipt_correction(cash_register_id);

CREATE INDEX idx_alcohol_product_detail ON alcohol_product_detail(product_id);
CREATE INDEX idx_alcohol_product_type ON alcohol_product_detail(alcohol_type_code);
CREATE INDEX idx_stamp_detail_stamp ON alcohol_excise_stamp_detail(excise_stamp_id);
CREATE INDEX idx_stamp_detail_full ON alcohol_excise_stamp_detail(stamp_full_code);
CREATE INDEX idx_stamp_detail_egais ON alcohol_excise_stamp_detail(egais_stamp_id);
CREATE INDEX idx_waybill_org ON egais_waybill(organization_id);
CREATE INDEX idx_waybill_number ON egais_waybill(waybill_number);
CREATE INDEX idx_waybill_egais ON egais_waybill(egais_document_id);
CREATE INDEX idx_waybill_date ON egais_waybill(waybill_date);
CREATE INDEX idx_waybill_type ON egais_waybill(waybill_type);
CREATE INDEX idx_waybill_item_waybill ON egais_waybill_item(waybill_id);
CREATE INDEX idx_waybill_item_product ON egais_waybill_item(product_id);
CREATE INDEX idx_writeoff_org ON egais_write_off_act(organization_id);
CREATE INDEX idx_writeoff_warehouse ON egais_write_off_act(warehouse_id);
CREATE INDEX idx_writeoff_egais ON egais_write_off_act(egais_document_id);
CREATE INDEX idx_writeoff_item_act ON egais_write_off_item(write_off_act_id);
CREATE INDEX idx_egais_balance_org ON egais_balance(organization_id);
CREATE INDEX idx_egais_balance_warehouse ON egais_balance(warehouse_id);
CREATE INDEX idx_egais_balance_product ON egais_balance(product_id);
CREATE INDEX idx_egais_queue_status ON egais_message_queue(status) WHERE status = 'PENDING';
CREATE INDEX idx_egais_queue_doc ON egais_message_queue(document_type, document_id);
CREATE INDEX idx_egais_journal_period ON egais_journal(journal_year, journal_month);
CREATE INDEX idx_egais_journal_product ON egais_journal(product_id);

-- ============================================================
-- 4. ФУНКЦИИ ДЛЯ РАБОТЫ С ЕГАИС
-- ============================================================

-- 4.1. Функция создания маршрутного листа
CREATE OR REPLACE FUNCTION create_egais_waybill(
    p_organization_id BIGINT,
    p_waybill_type VARCHAR,
    p_counterparty_id BIGINT,
    p_from_warehouse_id BIGINT,
    p_to_warehouse_id BIGINT,
    p_items JSONB
) RETURNS BIGINT AS $$
DECLARE
    v_waybill_id BIGINT;
    v_item JSONB;
    v_total_volume DECIMAL := 0;
    v_total_amount DECIMAL := 0;
    v_total_excise DECIMAL := 0;
BEGIN
    -- Генерируем номер маршрутного листа
    INSERT INTO egais_waybill (
        organization_id,
        waybill_number,
        waybill_date,
        waybill_type,
        counterparty_id,
        from_warehouse_id,
        to_warehouse_id,
        status
    ) VALUES (
        p_organization_id,
        'WB-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD(nextval('egais_waybill_seq')::TEXT, 6, '0'),
        CURRENT_DATE,
        p_waybill_type,
        p_counterparty_id,
        p_from_warehouse_id,
        p_to_warehouse_id,
        'DRAFT'
    ) RETURNING id INTO v_waybill_id;
    
    -- Добавляем позиции
    FOR v_item IN SELECT * FROM jsonb_array_elements(p_items)
    LOOP
        INSERT INTO egais_waybill_item (
            waybill_id,
            product_id,
            product_name,
            quantity,
            quantity_in_dal,
            quantity_in_bottles,
            alcohol_volume,
            bottle_volume,
            excise_rate,
            excise_amount,
            price,
            total_amount,
            excise_stamp_ids
        ) VALUES (
            v_waybill_id,
            (v_item->>'product_id')::BIGINT,
            v_item->>'product_name',
            (v_item->>'quantity')::DECIMAL,
            (v_item->>'quantity')::DECIMAL / 10,
            (v_item->>'quantity_in_bottles')::INT,
            (v_item->>'alcohol_volume')::DECIMAL,
            (v_item->>'bottle_volume')::DECIMAL,
            (v_item->>'excise_rate')::DECIMAL,
            (v_item->>'quantity')::DECIMAL * (v_item->>'excise_rate')::DECIMAL / 10,
            (v_item->>'price')::DECIMAL,
            (v_item->>'quantity')::DECIMAL * (v_item->>'price')::DECIMAL,
            ARRAY(SELECT jsonb_array_elements_text(v_item->'excise_stamp_ids'))::BIGINT[]
        );
        
        -- Суммируем
        v_total_volume := v_total_volume + (v_item->>'quantity')::DECIMAL;
        v_total_amount := v_total_amount + (v_item->>'quantity')::DECIMAL * (v_item->>'price')::DECIMAL;
        v_total_excise := v_total_excise + (v_item->>'quantity')::DECIMAL * (v_item->>'excise_rate')::DECIMAL / 10;
    END LOOP;
    
    -- Обновляем суммы в шапке
    UPDATE egais_waybill
    SET 
        total_volume = v_total_volume,
        total_amount = v_total_amount,
        total_excise = v_total_excise,
        updated_at = NOW()
    WHERE id = v_waybill_id;
    
    RETURN v_waybill_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION create_egais_waybill IS 'Создание маршрутного листа ЕГАИС';

-- 4.2. Функция списания алкоголя (перемещение в ЕГАИС)
CREATE OR REPLACE FUNCTION process_egais_write_off(
    p_organization_id BIGINT,
    p_warehouse_id BIGINT,
    p_write_off_type VARCHAR,
    p_reason TEXT,
    p_items JSONB
) RETURNS BIGINT AS $$
DECLARE
    v_act_id BIGINT;
    v_item JSONB;
BEGIN
    -- Создаем акт списания
    INSERT INTO egais_write_off_act (
        organization_id,
        warehouse_id,
        act_number,
        act_date,
        write_off_type,
        reason,
        status
    ) VALUES (
        p_organization_id,
        p_warehouse_id,
        'WO-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD(nextval('egais_writeoff_seq')::TEXT, 6, '0'),
        CURRENT_DATE,
        p_write_off_type,
        p_reason,
        'DRAFT'
    ) RETURNING id INTO v_act_id;
    
    -- Добавляем позиции
    FOR v_item IN SELECT * FROM jsonb_array_elements(p_items)
    LOOP
        INSERT INTO egais_write_off_item (
            write_off_act_id,
            product_id,
            product_name,
            quantity,
            quantity_in_dal,
            reason,
            excise_stamp_ids
        ) VALUES (
            v_act_id,
            (v_item->>'product_id')::BIGINT,
            v_item->>'product_name',
            (v_item->>'quantity')::DECIMAL,
            (v_item->>'quantity')::DECIMAL / 10,
            v_item->>'reason',
            ARRAY(SELECT jsonb_array_elements_text(v_item->'excise_stamp_ids'))::BIGINT[]
        );
    END LOOP;
    
    RETURN v_act_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION process_egais_write_off IS 'Создание акта списания алкоголя';

-- ============================================================
-- 5. МАТЕРИАЛИЗОВАННЫЕ ПРЕДСТАВЛЕНИЯ ДЛЯ АЛКОГОЛЬНОЙ ОТЧЕТНОСТИ
-- ============================================================

-- 5.1. Сводка по алкогольным декларациям
CREATE MATERIALIZED VIEW mv_alcohol_declaration_summary AS
SELECT 
    org.id AS organization_id,
    org.short_name AS organization_name,
    dec.declaration_year,
    dec.declaration_quarter,
    COUNT(DISTINCT dec.id) AS total_declarations,
    COUNT(DISTINCT dec.id) FILTER (WHERE dec.status = 'ACCEPTED') AS accepted_declarations,
    COUNT(DISTINCT dec.id) FILTER (WHERE dec.status = 'REJECTED') AS rejected_declarations,
    SUM(section.total_volume) AS total_volume,
    SUM(section.total_amount) AS total_amount,
    SUM(section.total_excise) AS total_excise
FROM organization org
JOIN alcohol_declaration dec ON org.id = dec.organization_id
JOIN alcohol_declaration_section section ON dec.id = section.declaration_id
GROUP BY org.id, org.short_name, dec.declaration_year, dec.declaration_quarter
WITH NO DATA;

CREATE UNIQUE INDEX idx_mv_alcohol_dec_summary ON mv_alcohol_declaration_summary(organization_id, declaration_year, declaration_quarter);

-- 5.2. Остатки алкоголя по складам (для ЕГАИС)
CREATE MATERIALIZED VIEW mv_egais_balance_summary AS
SELECT 
    org.id AS organization_id,
    org.short_name AS organization_name,
    w.id AS warehouse_id,
    w.name AS warehouse_name,
    p.id AS product_id,
    p.name AS product_name,
    p.gtin,
    eb.quantity,
    eb.quantity_in_dal,
    eb.batch_number,
    eb.production_date,
    eb.expiry_date,
    COUNT(DISTINCT ed.id) AS excise_stamps_count
FROM egais_balance eb
JOIN organization org ON eb.organization_id = org.id
JOIN warehouse w ON eb.warehouse_id = w.id
JOIN catalog_product p ON eb.product_id = p.id
LEFT JOIN alcohol_excise_stamp_detail ed ON ed.marking_code_id = p.id
GROUP BY org.id, org.short_name, w.id, w.name, p.id, p.name, p.gtin, 
         eb.quantity, eb.quantity_in_dal, eb.batch_number, eb.production_date, eb.expiry_date
WITH NO DATA;

CREATE INDEX idx_mv_egais_balance_org ON mv_egais_balance_summary(organization_id);
CREATE INDEX idx_mv_egais_balance_warehouse ON mv_egais_balance_summary(warehouse_id);