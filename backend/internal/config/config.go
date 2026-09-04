package config

import (
	"os"
	"strconv"
)

// Config читается из env. Значения по умолчанию — для локального dev на M2.
type Config struct {
	Env            string
	Port           string
	DatabaseURL    string
	RedisAddr      string
	JWTSecret      string
	JWTAccessTTL   int // минуты
	JWTRefreshDays int // дни
	SeedAdminEmail string
	SeedAdminPass  string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	ttl, _ := strconv.Atoi(getenv("JWT_ACCESS_TTL_MIN", "15"))
	if ttl <= 0 {
		ttl = 15
	}
	rd, _ := strconv.Atoi(getenv("JWT_REFRESH_TTL_DAYS", "7"))
	if rd <= 0 {
		rd = 7
	}
	return Config{
		Env:            getenv("APP_ENV", "dev"),
		Port:           getenv("BACKEND_PORT", "8080"),
		DatabaseURL:    getenv("DATABASE_URL", ""),
		RedisAddr:      getenv("REDIS_ADDR", ""),
		JWTSecret:      getenv("JWT_SECRET", "change-me-dev-secret-min-32-chars"),
		JWTAccessTTL:   ttl,
		JWTRefreshDays: rd,
		SeedAdminEmail: getenv("SEED_ADMIN_EMAIL", "admin@example.com"),
		SeedAdminPass:  getenv("SEED_ADMIN_PASSWORD", "admin123"),
	}
}
