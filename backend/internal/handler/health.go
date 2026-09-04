package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/store"
)

// Health регистрирует /healthz (liveness) и /readyz (зависимости).
func Health(e *echo.Echo, s *store.Store) {
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		pg := "disabled"
		rd := "disabled"
		if s.PG != nil {
			if err := s.PingPG(c.Request().Context()); err != nil {
				// PingPG возвращает nil-контекст ошибку когда пул nil — сюда не попадем
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "pg": "down"})
			}
			pg = "up"
		}
		if s.Redis != nil {
			if err := s.PingRedis(c.Request().Context()); err != nil {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "pg": pg, "redis": "down"})
			}
			rd = "up"
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready", "pg": pg, "redis": rd})
	})
	e.GET("/api/v1/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"pong": "ok"})
	})
}
