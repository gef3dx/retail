package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Store держит пулы. На этапе 1 подключение опционально:
// если DATABASE_URL/REDIS_ADDR пустые — сервер стартует без них,
// /readyz честно покажет degraded.
type Store struct {
	PG    *pgxpool.Pool
	Redis *redis.Client
}

func New(ctx context.Context, databaseURL, redisAddr string) (*Store, error) {
	s := &Store{}
	if databaseURL != "" {
		cfg, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			return nil, err
		}
		// Легкие лимиты для слабого ПК / M2 Air.
		cfg.MaxConns = 10
		cfg.MinConns = 1
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return nil, err
		}
		s.PG = pool
	}
	if redisAddr != "" {
		s.Redis = redis.NewClient(&redis.Options{
			Addr:        redisAddr,
			DialTimeout: 2 * time.Second,
		})
	}
	return s, nil
}

func (s *Store) PingPG(ctx context.Context) error {
	if s.PG == nil {
		return context.Cause(ctx) // nil-маркер: PG не настроен — обработаем выше
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.PG.Ping(ctx)
}

func (s *Store) PingRedis(ctx context.Context) error {
	if s.Redis == nil {
		return context.Cause(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.Redis.Ping(ctx).Err()
}

func (s *Store) Close() {
	if s.PG != nil {
		s.PG.Close()
	}
	if s.Redis != nil {
		_ = s.Redis.Close()
	}
}
