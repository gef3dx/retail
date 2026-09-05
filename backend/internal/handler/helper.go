package handler

import (
	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// writeErr мапит ошибку сервиса в JSON-ответ.
func writeErr(c echo.Context, err error) error {
	if se, ok := err.(*service.Error); ok {
		return c.JSON(se.Code, map[string]string{"error": se.Msg})
	}
	return c.JSON(500, map[string]string{"error": "internal error"})
}

func clientIP(c echo.Context) string {
	ip := c.RealIP()
	if ip == "::1" {
		return "127.0.0.1"
	}
	return ip
}

// ctxUser возвращает id текущего пользователя.
func ctxUser(c echo.Context) int64 {
	return middleware.CtxOf(c).UserID
}
