package config

import (
	"os"
	"strconv"
)

// Config читается из env. Значения по умолчанию — для локального dev на M2.
type Config struct {
	Env          string
	Port         string
	DatabaseURL  string
	RedisAddr    string
	JWTAccessTTL int // минуты
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
	return Config{
		Env:          getenv("APP_ENV", "dev"),
		Port:         getenv("BACKEND_PORT", "8080"),
		DatabaseURL:  getenv("DATABASE_URL", ""),
		RedisAddr:    getenv("REDIS_ADDR", ""),
		JWTAccessTTL: ttl,
	}
}
