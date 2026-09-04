.PHONY: up down logs ps migrate dev-be dev-fe check

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
	docker compose config

# Backend нативно (экономия RAM vs Docker)
dev-be:
	cd backend && go run ./cmd/api

# Frontend нативно
dev-fe:
	cd frontend && npm run dev

# Быстрые проверки этапа 1
check:
	cd backend && go vet ./... && go build ./...
	docker compose config -q && echo "compose OK"
