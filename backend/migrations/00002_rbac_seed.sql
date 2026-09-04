-- Этап 2: сиды ролей/разрешений (по sql/04, раздел 2). Идемпотентно через ON CONFLICT.

INSERT INTO roles (name, display_name, description, is_system) VALUES
('SUPER_ADMIN', 'Супер-администратор', 'Полный доступ ко всем функциям системы', TRUE),
('ADMIN', 'Администратор организации', 'Управление организацией и сотрудниками', TRUE),
('MANAGER', 'Менеджер', 'Управление заказами, товарами и отчетами', TRUE),
('CASHIER', 'Кассир', 'Работа с кассой и чеками', TRUE),
('ACCOUNTANT', 'Бухгалтер', 'Доступ к финансовым документам и отчетам', TRUE),
('WAREHOUSE_MANAGER', 'Кладовщик', 'Управление складом и инвентаризациями', TRUE),
('VIEWER', 'Наблюдатель', 'Только чтение отчетов', TRUE)
ON CONFLICT (name) DO NOTHING;

INSERT INTO permissions (code, name, description, resource, action) VALUES
('organization:create', 'Создание организации', 'Создание новой организации', 'organization', 'create'),
('organization:read', 'Просмотр организаций', 'Просмотр списка организаций', 'organization', 'read'),
('organization:update', 'Редактирование организации', 'Изменение данных организации', 'organization', 'update'),
('organization:delete', 'Удаление организации', 'Удаление организации (мягкое)', 'organization', 'delete'),
('product:create', 'Создание товара', 'Добавление нового товара', 'product', 'create'),
('product:read', 'Просмотр товаров', 'Просмотр списка товаров', 'product', 'read'),
('product:update', 'Редактирование товара', 'Изменение данных товара', 'product', 'update'),
('product:delete', 'Удаление товара', 'Удаление товара', 'product', 'delete'),
('document:create', 'Создание документа', 'Создание документов', 'document', 'create'),
('document:read', 'Просмотр документов', 'Просмотр документов', 'document', 'read'),
('document:update', 'Редактирование документа', 'Изменение документа', 'document', 'update'),
('document:delete', 'Удаление документа', 'Удаление документа', 'document', 'delete'),
('document:post', 'Проведение документа', 'Проведение документов', 'document', 'post'),
('receipt:create', 'Пробитие чека', 'Создание кассового чека', 'receipt', 'create'),
('receipt:read', 'Просмотр чеков', 'Просмотр кассовых чеков', 'receipt', 'read'),
('receipt:return', 'Возврат по чеку', 'Оформление возврата', 'receipt', 'return'),
('report:view', 'Просмотр отчетов', 'Просмотр отчетов и аналитики', 'report', 'view'),
('report:export', 'Экспорт отчетов', 'Выгрузка отчетов в Excel/PDF', 'report', 'export'),
('marking:view', 'Просмотр маркировки', 'Просмотр кодов маркировки', 'marking', 'view'),
('marking:manage', 'Управление маркировкой', 'Создание, перемещение кодов', 'marking', 'manage'),
('alcohol:view', 'Просмотр алкоголя', 'Просмотр алкогольной продукции', 'alcohol', 'view'),
('alcohol:manage', 'Управление алкоголем', 'Работа с ЕГАИС', 'alcohol', 'manage'),
('user:create', 'Создание пользователя', 'Создание учетных записей', 'user', 'create'),
('user:read', 'Просмотр пользователей', 'Просмотр списка пользователей', 'user', 'read'),
('user:update', 'Редактирование пользователя', 'Изменение данных пользователя', 'user', 'update'),
('user:delete', 'Удаление пользователя', 'Удаление пользователя', 'user', 'delete'),
('user:role', 'Управление ролями', 'Назначение ролей пользователям', 'user', 'role')
ON CONFLICT (code) DO NOTHING;

-- SUPER_ADMIN: все права
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'SUPER_ADMIN'
ON CONFLICT DO NOTHING;

-- ADMIN
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'ADMIN'
AND p.code IN ('organization:read','organization:update','product:create','product:read','product:update',
 'document:create','document:read','document:update','document:post','receipt:read',
 'report:view','report:export','marking:view','marking:manage','alcohol:view','alcohol:manage')
ON CONFLICT DO NOTHING;

-- MANAGER
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'MANAGER'
AND p.code IN ('product:read','product:update','document:create','document:read','document:update','document:post',
 'receipt:read','report:view','report:export','marking:view','alcohol:view')
ON CONFLICT DO NOTHING;

-- CASHIER: только касса
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'CASHIER'
AND p.code IN ('receipt:create','receipt:read','receipt:return','product:read','marking:view')
ON CONFLICT DO NOTHING;

-- ACCOUNTANT
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'ACCOUNTANT'
AND p.code IN ('document:read','document:post','receipt:read','report:view','report:export','alcohol:view')
ON CONFLICT DO NOTHING;

-- WAREHOUSE_MANAGER
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'WAREHOUSE_MANAGER'
AND p.code IN ('product:read','product:update','document:create','document:read','document:update','document:post',
 'marking:view','marking:manage')
ON CONFLICT DO NOTHING;

-- VIEWER
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'VIEWER'
AND p.code IN ('product:read','document:read','receipt:read','report:view','marking:view','alcohol:view')
ON CONFLICT DO NOTHING;
