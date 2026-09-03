-- ============================================================
-- 07_product_status.sql – Статусы товаров и услуг
-- ============================================================

-- ============================================================
-- 1. СПРАВОЧНИК СТАТУСОВ
-- ============================================================

CREATE TABLE product_status (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_terminal BOOLEAN DEFAULT FALSE,   -- финальный статус (например, 'DISCONTINUED')
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INT DEFAULT 0
);

COMMENT ON TABLE product_status IS 'Статусы товаров и услуг (активен, новинка, распродажа и т.д.)';

INSERT INTO product_status (code, name, description, is_terminal, sort_order) VALUES
('ACTIVE', 'Активный', 'Товар/услуга доступна для продажи', FALSE, 10),
('INACTIVE', 'Неактивный', 'Товар/услуга временно недоступна', FALSE, 20),
('NEW', 'Новинка', 'Новый товар/услуга на рынке', FALSE, 5),
('PROMOTION', 'Распродажа', 'Товар/услуга участвует в акции', FALSE, 15),
('SOON', 'Скоро в продаже', 'Товар/услуга появится позже', FALSE, 0),
('DISCONTINUED', 'Снят с продажи', 'Товар/услуга больше не продаётся', TRUE, 30),
('OUT_OF_STOCK', 'Нет в наличии', 'Товар временно отсутствует на складе', FALSE, 25),
('PREORDER', 'Предзаказ', 'Товар доступен для предзаказа', FALSE, 3);

-- ============================================================
-- 2. ДОБАВЛЯЕМ ПОЛЯ В ТАБЛИЦУ ТОВАРОВ
-- ============================================================

ALTER TABLE catalog_product ADD COLUMN status_code VARCHAR(20) DEFAULT 'ACTIVE' REFERENCES product_status(code);
COMMENT ON COLUMN catalog_product.status_code IS 'Статус товара/услуги';

ALTER TABLE catalog_product ADD COLUMN status_start_date DATE;
COMMENT ON COLUMN catalog_product.status_start_date IS 'Дата начала действия статуса';

ALTER TABLE catalog_product ADD COLUMN status_end_date DATE;
COMMENT ON COLUMN catalog_product.status_end_date IS 'Дата окончания действия статуса';

-- ============================================================
-- 3. ИНДЕКС
-- ============================================================

CREATE INDEX idx_product_status ON catalog_product(status_code) WHERE status_code IS NOT NULL;
CREATE INDEX idx_product_status_dates ON catalog_product(status_start_date, status_end_date);

-- ============================================================
-- 4. ФУНКЦИЯ ДЛЯ АВТОМАТИЧЕСКОГО ОБНОВЛЕНИЯ СТАТУСА ПО ДАТАМ
-- ============================================================

CREATE OR REPLACE FUNCTION update_product_status_by_dates()
RETURNS TRIGGER AS $$
BEGIN
    -- Если заданы даты начала и окончания, и текущая дата входит в интервал,
    -- но статус не совпадает с ожидаемым, можно автоматически установить статус.
    -- Это можно сделать через задание или триггер, но здесь просто пример.
    -- Для простоты оставляем ручное управление.
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- 5. ПРИМЕРЫ ИСПОЛЬЗОВАНИЯ (закомментированы)
-- ============================================================

-- -- Установка статуса для товара
-- UPDATE catalog_product
-- SET status_code = 'NEW',
--     status_start_date = CURRENT_DATE,
--     status_end_date = CURRENT_DATE + INTERVAL '30 days'
-- WHERE sku = 'PROD-001';
