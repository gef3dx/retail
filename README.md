# Retail Management System (RMS)

Система управления розничной торговлей для РФ: касса 54-ФЗ, маркировка «Честный знак», склад, заказы, услуги и бронирование, доставка, маркетплейсы, ЕГАИС, налоги, уведомления.

## Технологический стек (фактический)

| Компонент | Технология |
|-----------|------------|
| Backend | Go 1.25, Echo v4, pgx/v5 (без ORM), JWT (golang-jwt), go-redis |
| Frontend | React 18, TypeScript, Vite, Tailwind CSS, React Router v6, TanStack Query, Zustand, Axios |
| БД | PostgreSQL 14, Redis 7 |
| Инфра | Docker / Compose, nginx, Prometheus + Grafana, GitHub Actions |
| API | OpenAPI 3.0 (`docs/openapi.yaml`, 89 путей) |

Архитектура backend — послойная: `handler` (DTO/валидация) → `service` (транзакции, правила) → `repository` (только SQL) → `model` (сущности). SQL в хендлерах запрещён. Внешние системы — за интерфейсами провайдеров (`OFD_HTTP`, `GISMT_TRUEAPI`, SMTP, Telegram, СДЭК, Ozon/WB/Яндекс, УТМ); без ключей функции честно неактивны, в dev работают эмуляторы.

## Требования

Go 1.25+, Node.js 18+, Docker + Compose plugin, Make.

## Быстрый старт (разработка)

```bash
cp .env.example .env   # при необходимости поправьте пароли
make up                # postgres + redis
make migrate           # 16 миграций (идемпотентны)
make dev-be            # backend :8080 (терминал 1)
make dev-fe            # frontend :5173 (терминал 2)
```

Откройте http://localhost:5173, зарегистрируйте организацию
или войдите супер-админом (создаётся при старте из `SEED_ADMIN_EMAIL` / `SEED_ADMIN_PASSWORD`, по умолчанию `admin@example.com / admin123`).

Проверка связи:

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/metrics | head
```

## Конфигурация (.env)

| Переменная | Назначение | Дефолт |
|------------|------------|--------|
| `DATABASE_URL` | DSN Postgres | `postgres://retail:retail@localhost:5432/retail?sslmode=disable` |
| `REDIS_ADDR` | Redis | `localhost:6379` |
| `BACKEND_PORT` | Порт API | `8080` |
| `JWT_SECRET` | Подпись токенов (мин. 32 символа!) | dev-значение |
| `JWT_ACCESS_TTL_MIN` / `JWT_REFRESH_TTL_DAYS` | Время жизни токенов | `15` / `7` |
| `SETTINGS_ENC_KEY` | AES-ключ шифрования секретов интеграций (base64, 32 байта). Сгенерировать: `openssl rand -base64 32`. Без него — dev-ключ с предупреждением, **не для продакшена** | — |
| `SEED_ADMIN_EMAIL` / `SEED_ADMIN_PASSWORD` | Сид супер-админа | `admin@example.com / admin123` |
| `OFD_WORKER_INTERVAL_SEC` / `BOOKING_REMIND_SEC` | Интервалы воркеров | `2` / `60` |
| `VITE_API_URL` | URL API для фронта | `http://localhost:8080/api/v1` |

## Структура проекта

```
backend/
  cmd/api/            точка входа, роуты (120 эндпоинтов), воркеры
  internal/
    handler/          тонкие HTTP-хендлеры
    service/          бизнес-логика и транзакции
    repository/       SQL (DBTX: работает и в транзакции, и вне)
    model/            сущности
    provider/         реестр провайдеров интеграций + AES-шифрование секретов
    ofd/ gismt/ notify/ booking/ delivery/ market/ egais/  провайдеры и воркеры
    metrics/          счётчики/гистограммы Prometheus без зависимостей
    middleware/       JWT + RBAC
  migrations/         00001–00016, embedded, идемпотентны
  Dockerfile          мультистадийный (api + migrate, ~40MB)
frontend/
  src/pages/          Login, Products, Cashier, Stock, Orders, Delivery,
                      Bookings, Services, Marking, Notify, Reports,
                      Marketplaces, Egais, Integrations, Users, Me ...
  Dockerfile          build + nginx SPA
docs/
  openapi.yaml        OpenAPI 3.0, все 89 путей + x-permission на каждый
  *.md                исходные проектные заметки и SQL-схема (sql/)
deploy/
  nginx.conf          edge-proxy: TLS, /api → backend, / → frontend
  prometheus.yml      scrape /metrics
  grafana-*.yml/json  datasource + дашборд RPS/5xx/p95
scripts/backup.sh     pg_dump + ротация (cron-пример внутри)
```

## API и права

Полный перечень — `docs/openapi.yaml` (Swagger UI: `make swagger` → http://localhost:8081).
Каждый путь помечен `x-permission`. Базовые роли: `SUPER_ADMIN`, `ADMIN`, `MANAGER`, `CASHIER`, `ACCOUNTANT`, `WAREHOUSE_MANAGER`, `VIEWER`. Ключевые права: `receipt:create` (касса), `document:*` (склад/заказы), `marking:*`, `alcohol:*` (ЕГАИС), `report:view/export` (налоги), `organization:update` (настройки, интеграции).

## Make-цели

| Цель | Действие |
|------|----------|
| `make up/down/ps/logs` | Инфра dev |
| `make migrate` | Миграции |
| `make dev-be/dev-fe` | Запуск нативно |
| `make check/ci/fmt` | vet+build / полный CI / форматирование |
| `make swagger` | Swagger UI |
| `make monitoring` | Prometheus + Grafana |
| `make backup` | Бэкап PG |
| `make prod-build/prod-up/prod-logs` | Продакшен |

## Production

```bash
cp .env.example .env   # ОБЯЗАТЕЛЬНО задайте: POSTGRES_PASSWORD, JWT_SECRET,
                       # SETTINGS_ENC_KEY, SEED_ADMIN_PASSWORD, GRAFANA_PASSWORD
mkdir -p deploy/tls deploy/certbot-www
# положите fullchain.pem/privkey.pem в deploy/tls/ (или получите через certbot)
docker compose -f docker-compose.prod.yml up -d --build
```

Состав: `migrate` (init) → `backend` + `frontend` → `nginx` (:80/:443).
Мониторинг: добавьте `--profile monitoring`. Бэкапы: `./scripts/backup.sh` по cron.

## Ограничения (честно)

- ОФД/ГИС МТ/маркетплейсы/СДЭК/УТМ работают по-настоящему только с договорами и ключами (вводятся в «Интеграциях»); без них — эмуляторы (dev) или `INACTIVE`.
- SMS/PUSH/WhatsApp — универсальный HTTP-шлюз на URL из настроек.
- Экспорт отчётов — CSV (открывается в Excel); PDF — через печать страницы.
- ЭДО/электронная подпись — не реализованы (обмен УПД вручную).
- Партиционирование и матпредставления из исходного ТЗ отложены (индексы есть).

## Лицензия

MIT
