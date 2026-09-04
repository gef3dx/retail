# Backend — Retail Management System (Этап 1: скелет)

Go 1.22+, Echo, pgx, go-redis. Запуск нативно (без Docker) для экономии RAM.

```bash
cp ../.env.example ../.env   # или export DATABASE_URL=... REDIS_ADDR=...
go run ./cmd/api             # :8080
curl localhost:8080/healthz
curl localhost:8080/readyz
```
