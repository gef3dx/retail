-- ============================================================
-- 04_users_security.sql – Система пользователей, ролей и прав доступа
-- ============================================================

-- ============================================================
-- 1. ПОЛЬЗОВАТЕЛИ
-- ============================================================

-- 1.1. Пользователи системы
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    
    -- Учетные данные
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    salt VARCHAR(64),
    
    -- Личная информация
    first_name VARCHAR(50) NOT NULL,
    last_name VARCHAR(50) NOT NULL,
    middle_name VARCHAR(50),
    phone VARCHAR(20),
    
    -- Статус
    is_active BOOLEAN DEFAULT TRUE,
    is_locked BOOLEAN DEFAULT FALSE,
    locked_until TIMESTAMP,
    failed_login_attempts INT DEFAULT 0,
    last_login_at TIMESTAMP,
    last_login_ip INET,
    
    -- Приглашение
    invited_by_id BIGINT REFERENCES users(id),
    invitation_token VARCHAR(64),
    invitation_sent_at TIMESTAMP,
    invitation_accepted_at TIMESTAMP,
    
    -- Настройки (JSON)
    preferences JSONB DEFAULT '{}'::JSONB,
    
    -- Метаданные
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

COMMENT ON TABLE users IS 'Пользователи системы (учетные записи)';
COMMENT ON COLUMN users.password_hash IS 'Хеш пароля, рекомендуется использовать bcrypt с солью';
COMMENT ON COLUMN users.preferences IS 'Настройки пользователя (тема, язык, уведомления и т.д.)';

-- 1.2. Связь пользователя с сотрудником (расширяем таблицу employee)
ALTER TABLE employee ADD COLUMN user_id BIGINT UNIQUE REFERENCES users(id);
ALTER TABLE employee ADD COLUMN is_system_user BOOLEAN DEFAULT FALSE;

COMMENT ON COLUMN employee.user_id IS 'Ссылка на учетную запись пользователя (если сотрудник имеет доступ к системе)';
COMMENT ON COLUMN employee.is_system_user IS 'Флаг, что сотрудник является системным пользователем (имеет логин)';

-- Удаляем старые поля login и password_hash из employee, если они есть
-- (можно оставить для обратной совместимости, но лучше перенести)
-- ALTER TABLE employee DROP COLUMN IF EXISTS login;
-- ALTER TABLE employee DROP COLUMN IF EXISTS password_hash;

-- 1.3. Сессии пользователей (для JWT или cookie-based)
CREATE TABLE user_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Токены
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    access_token_expires_at TIMESTAMP NOT NULL,
    refresh_token_expires_at TIMESTAMP,
    
    -- Данные сессии
    ip_address INET,
    user_agent TEXT,
    device_name VARCHAR(100),
    
    -- Статус
    is_active BOOLEAN DEFAULT TRUE,
    terminated_at TIMESTAMP,
    terminated_by VARCHAR(50),
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE user_sessions IS 'Активные сессии пользователей (для управления токенами)';

CREATE INDEX idx_sessions_user ON user_sessions(user_id);
CREATE INDEX idx_sessions_access_token ON user_sessions(access_token);
CREATE INDEX idx_sessions_refresh_token ON user_sessions(refresh_token);
CREATE INDEX idx_sessions_active ON user_sessions(is_active) WHERE is_active = TRUE;

-- 1.4. Журнал действий пользователей (аудит)
CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id),
    
    -- Тип действия
    action_type VARCHAR(50) NOT NULL,
    action_description TEXT,
    
    -- Сущность, над которой совершено действие
    entity_type VARCHAR(50),
    entity_id BIGINT,
    
    -- Данные до/после (JSON)
    old_values JSONB,
    new_values JSONB,
    
    -- Метаданные
    ip_address INET,
    user_agent TEXT,
    session_id BIGINT REFERENCES user_sessions(id),
    
    -- Статус выполнения
    is_success BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    
    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE audit_log IS 'Журнал аудита всех действий пользователей';

CREATE INDEX idx_audit_user ON audit_log(user_id);
CREATE INDEX idx_audit_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_action ON audit_log(action_type);
CREATE INDEX idx_audit_created ON audit_log(created_at);

-- ============================================================
-- 2. РОЛИ И РАЗРЕШЕНИЯ (RBAC)
-- ============================================================

-- 2.1. Роли
CREATE TABLE roles (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    description TEXT,
    is_system BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE roles IS 'Роли пользователей (администратор, менеджер, кассир и т.д.)';

-- Базовые роли (системные)
INSERT INTO roles (name, display_name, description, is_system) VALUES
('SUPER_ADMIN', 'Супер-администратор', 'Полный доступ ко всем функциям системы', TRUE),
('ADMIN', 'Администратор организации', 'Управление организацией и сотрудниками', TRUE),
('MANAGER', 'Менеджер', 'Управление заказами, товарами и отчетами', TRUE),
('CASHIER', 'Кассир', 'Работа с кассой и чеками', TRUE),
('ACCOUNTANT', 'Бухгалтер', 'Доступ к финансовым документам и отчетам', TRUE),
('WAREHOUSE_MANAGER', 'Кладовщик', 'Управление складом и инвентаризациями', TRUE),
('VIEWER', 'Наблюдатель', 'Только чтение отчетов', TRUE);

-- 2.2. Разрешения (права доступа)
CREATE TABLE permissions (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    resource VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE permissions IS 'Разрешения (права доступа) для ролей';

-- Добавим базовые разрешения (можно расширять)
INSERT INTO permissions (code, name, description, resource, action) VALUES
-- Организации
('organization:create', 'Создание организации', 'Создание новой организации', 'organization', 'create'),
('organization:read', 'Просмотр организаций', 'Просмотр списка организаций', 'organization', 'read'),
('organization:update', 'Редактирование организации', 'Изменение данных организации', 'organization', 'update'),
('organization:delete', 'Удаление организации', 'Удаление организации (мягкое)', 'organization', 'delete'),
-- Сотрудники
('employee:create', 'Создание сотрудника', 'Добавление нового сотрудника', 'employee', 'create'),
('employee:read', 'Просмотр сотрудников', 'Просмотр списка сотрудников', 'employee', 'read'),
('employee:update', 'Редактирование сотрудника', 'Изменение данных сотрудника', 'employee', 'update'),
('employee:delete', 'Удаление сотрудника', 'Удаление сотрудника', 'employee', 'delete'),
-- Товары
('product:create', 'Создание товара', 'Добавление нового товара', 'product', 'create'),
('product:read', 'Просмотр товаров', 'Просмотр списка товаров', 'product', 'read'),
('product:update', 'Редактирование товара', 'Изменение данных товара', 'product', 'update'),
('product:delete', 'Удаление товара', 'Удаление товара', 'product', 'delete'),
-- Документы (поступления, отгрузки)
('document:create', 'Создание документа', 'Создание документов поступления/отгрузки', 'document', 'create'),
('document:read', 'Просмотр документов', 'Просмотр документов', 'document', 'read'),
('document:update', 'Редактирование документа', 'Изменение документа', 'document', 'update'),
('document:delete', 'Удаление документа', 'Удаление документа', 'document', 'delete'),
('document:post', 'Проведение документа', 'Проведение документов', 'document', 'post'),
-- Касса
('receipt:create', 'Пробитие чека', 'Создание кассового чека', 'receipt', 'create'),
('receipt:read', 'Просмотр чеков', 'Просмотр кассовых чеков', 'receipt', 'read'),
('receipt:return', 'Возврат по чеку', 'Оформление возврата', 'receipt', 'return'),
-- Отчеты
('report:view', 'Просмотр отчетов', 'Просмотр отчетов и аналитики', 'report', 'view'),
('report:export', 'Экспорт отчетов', 'Выгрузка отчетов в Excel/PDF', 'report', 'export'),
-- Маркировка (ЧЗ)
('marking:view', 'Просмотр маркировки', 'Просмотр кодов маркировки', 'marking', 'view'),
('marking:manage', 'Управление маркировкой', 'Создание, перемещение кодов', 'marking', 'manage'),
-- Алкоголь
('alcohol:view', 'Просмотр алкоголя', 'Просмотр алкогольной продукции', 'alcohol', 'view'),
('alcohol:manage', 'Управление алкоголем', 'Работа с алкогольными декларациями, ЕГАИС', 'alcohol', 'manage'),
-- Пользователи
('user:create', 'Создание пользователя', 'Создание учетных записей', 'user', 'create'),
('user:read', 'Просмотр пользователей', 'Просмотр списка пользователей', 'user', 'read'),
('user:update', 'Редактирование пользователя', 'Изменение данных пользователя', 'user', 'update'),
('user:delete', 'Удаление пользователя', 'Удаление пользователя', 'user', 'delete'),
('user:role', 'Управление ролями', 'Назначение ролей пользователям', 'user', 'role');

-- 2.3. Связь ролей и разрешений (многие ко многим)
CREATE TABLE role_permissions (
    id BIGSERIAL PRIMARY KEY,
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (role_id, permission_id)
);

COMMENT ON TABLE role_permissions IS 'Назначение разрешений ролям';

-- Назначаем разрешения для базовых ролей (пример)
-- SUPER_ADMIN - все права
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'SUPER_ADMIN';

-- ADMIN - управление организацией, сотрудниками, товарами, документами, но без управления пользователями и системой
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'ADMIN'
AND p.code IN (
    'organization:read', 'organization:update',
    'employee:create', 'employee:read', 'employee:update',
    'product:create', 'product:read', 'product:update',
    'document:create', 'document:read', 'document:update', 'document:post',
    'receipt:read', 'report:view', 'report:export',
    'marking:view', 'marking:manage',
    'alcohol:view', 'alcohol:manage'
);

-- MANAGER - управление документами и заказами, просмотр всего
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'MANAGER'
AND p.code IN (
    'product:read', 'product:update',
    'document:create', 'document:read', 'document:update', 'document:post',
    'receipt:read',
    'report:view', 'report:export',
    'marking:view',
    'alcohol:view'
);

-- CASHIER - только касса и чеки
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'CASHIER'
AND p.code IN (
    'receipt:create', 'receipt:read', 'receipt:return',
    'product:read',
    'marking:view'
);

-- ACCOUNTANT - финансы, отчеты, документы
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'ACCOUNTANT'
AND p.code IN (
    'document:read', 'document:post',
    'receipt:read',
    'report:view', 'report:export',
    'alcohol:view'
);

-- WAREHOUSE_MANAGER - склад
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'WAREHOUSE_MANAGER'
AND p.code IN (
    'product:read', 'product:update',
    'document:create', 'document:read', 'document:update', 'document:post',
    'marking:view', 'marking:manage'
);

-- VIEWER - только чтение отчетов и просмотр данных
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'VIEWER'
AND p.code IN (
    'product:read',
    'document:read',
    'receipt:read',
    'report:view',
    'marking:view',
    'alcohol:view'
);

-- 2.4. Назначение ролей пользователям (многие ко многим)
CREATE TABLE user_roles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    organization_id BIGINT REFERENCES organization(id),
    granted_by_id BIGINT REFERENCES users(id),
    granted_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    UNIQUE (user_id, role_id, organization_id)
);

COMMENT ON TABLE user_roles IS 'Назначение ролей пользователям (возможно, в разрезе организаций)';

CREATE INDEX idx_user_roles_user ON user_roles(user_id);
CREATE INDEX idx_user_roles_org ON user_roles(organization_id);

-- 2.5. Связь пользователя с организациями (доступ к нескольким организациям)
CREATE TABLE user_organizations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    is_default BOOLEAN DEFAULT FALSE,
    joined_at TIMESTAMP DEFAULT NOW(),
    left_at TIMESTAMP,
    UNIQUE (user_id, organization_id)
);

COMMENT ON TABLE user_organizations IS 'Привязка пользователей к организациям (пользователь может работать в нескольких)';

CREATE INDEX idx_user_org_user ON user_organizations(user_id);
CREATE INDEX idx_user_org_org ON user_organizations(organization_id);

-- ============================================================
-- 3. ПРИГЛАШЕНИЯ И РЕГИСТРАЦИЯ
-- ============================================================

-- 3.1. Таблица приглашений (для регистрации новых пользователей)
CREATE TABLE invitations (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT REFERENCES organization(id),
    invited_by_id BIGINT REFERENCES users(id),
    
    -- Кому отправлено
    email VARCHAR(100) NOT NULL,
    phone VARCHAR(20),
    
    -- Роли, которые будут назначены при регистрации (массив ID ролей)
    role_ids BIGINT[],
    
    -- Токен приглашения
    token VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    
    -- Статус
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'ACCEPTED', 'EXPIRED', 'CANCELED')),
    
    -- Дополнительные данные
    message TEXT,
    metadata JSONB,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    accepted_at TIMESTAMP
);

COMMENT ON TABLE invitations IS 'Приглашения для регистрации новых пользователей';

CREATE INDEX idx_invitations_token ON invitations(token);
CREATE INDEX idx_invitations_email ON invitations(email);
CREATE INDEX idx_invitations_status ON invitations(status);

-- ============================================================
-- 4. ИНДЕКСЫ ДЛЯ НОВЫХ ТАБЛИЦ
-- ============================================================

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_active ON users(is_active) WHERE is_active = TRUE;

CREATE INDEX idx_employee_user ON employee(user_id) WHERE user_id IS NOT NULL;

-- ============================================================
-- 5. ТРИГГЕРЫ ДЛЯ ОБНОВЛЕНИЯ UPDATED_AT
-- ============================================================

DO $$
DECLARE
    table_name text;
BEGIN
    FOR table_name IN 
        SELECT tablename 
        FROM pg_tables 
        WHERE schemaname = 'public' 
        AND tablename IN ('users', 'user_sessions', 'roles', 'permissions', 'role_permissions', 'user_roles', 'user_organizations', 'invitations')
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
-- 6. ФУНКЦИИ ДЛЯ РАБОТЫ С ПОЛЬЗОВАТЕЛЯМИ И ОРГАНИЗАЦИЯМИ
-- ============================================================

-- 6.1. Создание пользователя и автоматическое назначение роли
CREATE OR REPLACE FUNCTION create_user_with_organization(
    p_username VARCHAR,
    p_email VARCHAR,
    p_password_hash VARCHAR,
    p_first_name VARCHAR,
    p_last_name VARCHAR,
    p_organization_name VARCHAR,
    p_organization_inn VARCHAR,
    p_organization_kpp VARCHAR,
    p_role_name VARCHAR DEFAULT 'ADMIN'
) RETURNS JSONB AS $$
DECLARE
    v_user_id BIGINT;
    v_org_id BIGINT;
    v_role_id BIGINT;
    v_result JSONB;
BEGIN
    -- 1. Создаем организацию
    INSERT INTO organization (inn, kpp, full_name, short_name, legal_address, tax_system, is_active)
    VALUES (
        p_organization_inn,
        p_organization_kpp,
        p_organization_name,
        p_organization_name,
        'Адрес не указан',
        'OSN',
        TRUE
    ) RETURNING id INTO v_org_id;
    
    -- 2. Создаем пользователя
    INSERT INTO users (username, email, password_hash, first_name, last_name, is_active)
    VALUES (
        p_username,
        p_email,
        p_password_hash,
        p_first_name,
        p_last_name,
        TRUE
    ) RETURNING id INTO v_user_id;
    
    -- 3. Привязываем пользователя к организации
    INSERT INTO user_organizations (user_id, organization_id, is_default)
    VALUES (v_user_id, v_org_id, TRUE);
    
    -- 4. Назначаем роль
    SELECT id INTO v_role_id FROM roles WHERE name = p_role_name;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Роль % не найдена', p_role_name;
    END IF;
    
    INSERT INTO user_roles (user_id, role_id, organization_id, granted_by_id)
    VALUES (v_user_id, v_role_id, v_org_id, NULL);
    
    -- 5. Создаем сотрудника (если нужно)
    INSERT INTO employee (organization_id, last_name, first_name, user_id, is_system_user)
    VALUES (v_org_id, p_last_name, p_first_name, v_user_id, TRUE);
    
    -- Возвращаем результат
    v_result := jsonb_build_object(
        'user_id', v_user_id,
        'organization_id', v_org_id,
        'username', p_username,
        'role', p_role_name
    );
    
    RETURN v_result;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION create_user_with_organization IS 'Создание пользователя вместе с организацией (для регистрации)';

-- 6.2. Функция добавления сотрудника с созданием пользователя (если нужно)
CREATE OR REPLACE FUNCTION add_employee_with_user(
    p_organization_id BIGINT,
    p_last_name VARCHAR,
    p_first_name VARCHAR,
    p_middle_name VARCHAR DEFAULT NULL,
    p_position VARCHAR DEFAULT NULL,
    p_username VARCHAR DEFAULT NULL,
    p_email VARCHAR DEFAULT NULL,
    p_password_hash VARCHAR DEFAULT NULL,
    p_role_name VARCHAR DEFAULT 'VIEWER'
) RETURNS BIGINT AS $$
DECLARE
    v_employee_id BIGINT;
    v_user_id BIGINT;
    v_role_id BIGINT;
BEGIN
    -- Если передан username, создаем пользователя
    IF p_username IS NOT NULL AND p_email IS NOT NULL AND p_password_hash IS NOT NULL THEN
        -- Проверяем, не занят ли username/email
        IF EXISTS (SELECT 1 FROM users WHERE username = p_username OR email = p_email) THEN
            RAISE EXCEPTION 'Пользователь с таким логином или email уже существует';
        END IF;
        
        INSERT INTO users (username, email, password_hash, first_name, last_name, middle_name, is_active)
        VALUES (p_username, p_email, p_password_hash, p_first_name, p_last_name, p_middle_name, TRUE)
        RETURNING id INTO v_user_id;
        
        -- Привязываем к организации
        INSERT INTO user_organizations (user_id, organization_id, is_default)
        VALUES (v_user_id, p_organization_id, TRUE);
        
        -- Назначаем роль
        SELECT id INTO v_role_id FROM roles WHERE name = p_role_name;
        IF NOT FOUND THEN
            v_role_id := (SELECT id FROM roles WHERE name = 'VIEWER');
        END IF;
        
        INSERT INTO user_roles (user_id, role_id, organization_id)
        VALUES (v_user_id, v_role_id, p_organization_id);
    END IF;
    
    -- Создаем сотрудника
    INSERT INTO employee (
        organization_id,
        last_name,
        first_name,
        middle_name,
        position,
        user_id,
        is_system_user,
        is_active
    ) VALUES (
        p_organization_id,
        p_last_name,
        p_first_name,
        p_middle_name,
        p_position,
        v_user_id,
        (v_user_id IS NOT NULL),
        TRUE
    ) RETURNING id INTO v_employee_id;
    
    RETURN v_employee_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION add_employee_with_user IS 'Добавление сотрудника с возможностью создания учетной записи';

-- ============================================================
-- 7. МАТЕРИАЛИЗОВАННЫЕ ПРЕДСТАВЛЕНИЯ ДЛЯ АДМИНИСТРИРОВАНИЯ
-- ============================================================

-- 7.1. Список пользователей с их ролями и организациями
CREATE MATERIALIZED VIEW mv_users_roles_orgs AS
SELECT 
    u.id AS user_id,
    u.username,
    u.email,
    u.first_name || ' ' || u.last_name AS full_name,
    u.is_active,
    u.last_login_at,
    STRING_AGG(DISTINCT r.name, ', ') AS role_names,
    STRING_AGG(DISTINCT o.short_name, ', ') AS organization_names,
    COUNT(DISTINCT uo.organization_id) AS organization_count
FROM users u
LEFT JOIN user_roles ur ON u.id = ur.user_id
LEFT JOIN roles r ON ur.role_id = r.id
LEFT JOIN user_organizations uo ON u.id = uo.user_id
LEFT JOIN organization o ON uo.organization_id = o.id
GROUP BY u.id, u.username, u.email, u.first_name, u.last_name, u.is_active, u.last_login_at
WITH NO DATA;

CREATE UNIQUE INDEX idx_mv_users_roles_orgs ON mv_users_roles_orgs(user_id);

COMMENT ON MATERIALIZED VIEW mv_users_roles_orgs IS 'Сводка по пользователям с их ролями и организациями';

-- ============================================================
-- 8. НАЧАЛЬНЫЕ ДАННЫЕ (СУПЕР-АДМИН)
-- ============================================================

-- Создаем супер-администратора (пароль: admin123, хеш нужно сгенерировать отдельно)
-- В реальном проекте пароль должен быть хеширован через bcrypt.
-- Здесь пример с простым хешем для демонстрации.
INSERT INTO users (username, email, password_hash, first_name, last_name, is_active)
VALUES (
    'superadmin',
    'admin@example.com',
    -- Это пример хеша для 'admin123' (небезопасно, замените на реальный bcrypt)
    '$2a$10$7XUu5xI8y3kR1AqZ1XZ1XeZ1XZ1XZ1XZ1XZ1XZ1XZ1XZ1XZ1XZ1X',
    'Super',
    'Admin',
    TRUE
) ON CONFLICT (username) DO NOTHING;

-- Назначаем супер-админу роль SUPER_ADMIN (если пользователь создан)
DO $$
DECLARE
    v_user_id BIGINT;
    v_role_id BIGINT;
BEGIN
    SELECT id INTO v_user_id FROM users WHERE username = 'superadmin';
    SELECT id INTO v_role_id FROM roles WHERE name = 'SUPER_ADMIN';
    IF v_user_id IS NOT NULL AND v_role_id IS NOT NULL THEN
        INSERT INTO user_roles (user_id, role_id) VALUES (v_user_id, v_role_id)
        ON CONFLICT (user_id, role_id, organization_id) DO NOTHING;
    END IF;
END;
$$;

COMMENT ON TABLE users IS 'Пользователи системы';

-- ============================================================
-- 9. ПРИМЕРЫ ИСПОЛЬЗОВАНИЯ (закомментированы)
-- ============================================================

-- -- Создание пользователя с организацией
-- SELECT create_user_with_organization(
--     'ivanov',
--     'ivanov@company.ru',
--     '$2a$10$...', -- хеш пароля
--     'Иван',
--     'Иванов',
--     'ООО "Ромашка"',
--     '1234567890',
--     '770101001',
--     'ADMIN'
-- );

-- -- Добавление сотрудника с созданием пользователя
-- SELECT add_employee_with_user(
--     1, -- organization_id
--     'Петров',
--     'Петр',
--     'Сергеевич',
--     'Менеджер',
--     'petrov',
--     'petrov@company.ru',
--     '$2a$10$...', -- хеш пароля
--     'MANAGER'
-- );