package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/auth"
	"retail-backend/internal/store"
)

type Ctx struct {
	UserID   int64
	Username string
	Roles    []string
	Perms    map[string]bool
}

// AuthJWT проверяет Bearer-токен, кладет Ctx в контекст, подтягивает права из БД.
func AuthJWT(secret string, s *store.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing token"})
			}
			cl, err := auth.ParseAccessToken(secret, strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}
			perms, err := s.Permissions(c.Request().Context(), cl.UserID)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "perms lookup failed"})
			}
			c.Set("ctx", &Ctx{UserID: cl.UserID, Username: cl.Username, Roles: cl.Roles, Perms: perms})
			return next(c)
		}
	}
}

func CtxOf(c echo.Context) *Ctx {
	if v, ok := c.Get("ctx").(*Ctx); ok {
		return v
	}
	return &Ctx{Perms: map[string]bool{}}
}

func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}

// RequirePermission: SUPER_ADMIN пропускает всегда.
func RequirePermission(code string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			x := CtxOf(c)
			if hasRole(x.Roles, "SUPER_ADMIN") || x.Perms[code] {
				return next(c)
			}
			return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden: need " + code})
		}
	}
}
