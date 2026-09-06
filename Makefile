.PHONY: up down logs ps migrate dev-be dev-fe check seed ci prod-build prod-up backup swagger monitoring fmt

# Поднять только инфру (PG + Redis) — легко для M2
up:
	docker compose up -d pg redis

down:
	docker compose down

logs:
	docker compose logs -f pg redis

ps:
	docker compose ps

# Проверка конфига compose
config:
	docker compose config -q && echo "dev compose OK"
	GRAFANA_PASSWORD=dummy POSTGRES_PASSWORD=dummy JWT_SECRET=dummy-32-chars-secret-1234567890 SETTINGS_ENC_KEY=x SEED_ADMIN_PASSWORD=x docker compose -f docker-compose.prod.yml config -q && echo "prod compose OK"

# Backend нативно (экономия RAM vs Docker)
dev-be:
	cd backend && go run ./cmd/api

# Frontend нативно
dev-fe:
	cd frontend && npm run dev

# Быстрые проверки
check:
	cd backend && go vet ./... && go build ./...
	docker compose config -q && echo "compose OK"

# Полный локальный CI (как в GitHub Actions, без smoke-стенда)
ci:
	cd backend && gofmt -l ./... | tee /tmp/fmt.out && test ! -s /tmp/fmt.out
	cd backend && go vet ./... && go build ./...
	cd frontend && npm run build

# Миграции (нужен DATABASE_URL, см. .env.example)
migrate:
	cd backend && DATABASE_URL=$${DATABASE_URL:-postgres://retail:retail@localhost:5432/retail?sslmode=disable} go run ./migrations

# Подсказка по сиду админа (создается автоматически при старте backend)
seed:
	@echo "Seed выполняется при старте backend: SEED_ADMIN_EMAIL / SEED_ADMIN_PASSWORD"

# Форматирование
fmt:
	cd backend && gofmt -w ./... && go vet ./...

# Production: сборка и запуск
prod-build:
	docker compose -f docker-compose.prod.yml build

prod-up:
	docker compose -f docker-compose.prod.yml up -d

prod-logs:
	docker compose -f docker-compose.prod.yml logs -f backend frontend nginx

# Бэкап PG (см. scripts/backup.sh)
backup:
	./scripts/backup.sh

# Swagger UI для docs/openapi.yaml (http://localhost:8081)
swagger:
	docker compose --profile docs up -d swagger

# Мониторинг (Prometheus :9090 внутри сети, Grafana :3000)
monitoring:
	docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile monitoring up -d prometheus grafana
