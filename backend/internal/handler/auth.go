package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/service"
)

// Auth — тонкий слой: DTO + вызов AuthService.
type Auth struct {
	Svc *service.AuthService
}

func (a *Auth) Register(c echo.Context) error {
	var r service.RegisterInput
	if err := c.Bind(&r); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	// Маппим snake_case JSON на структуру (Echo bind чувствителен к регистру тегов по умолчанию? нет — нечувствителен).
	tokens, err := a.Svc.Register(c.Request().Context(), r, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusCreated, tokens)
}

func (a *Auth) Login(c echo.Context) error {
	var r struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := c.Bind(&r); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	tokens, err := a.Svc.Login(c.Request().Context(), r.Login, r.Password, clientIP(c), c.Request().UserAgent())
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, tokens)
}

func (a *Auth) Refresh(c echo.Context) error {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	tokens, err := a.Svc.Refresh(c.Request().Context(), body.RefreshToken)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, tokens)
}

func (a *Auth) Logout(c echo.Context) error {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.Bind(&body)
	a.Svc.Logout(c.Request().Context(), body.RefreshToken)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (a *Auth) Me(c echo.Context) error {
	x := middleware.CtxOf(c)
	u, err := a.Svc.Me(c.Request().Context(), x.UserID, x.Roles)
	if err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, u)
}

func (a *Auth) PatchMe(c echo.Context) error {
	x := middleware.CtxOf(c)
	var b struct {
		Telegram *string `json:"telegram_chat_id"`
		Push     *string `json:"push_token"`
	}
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if err := a.Svc.UpdateMe(c.Request().Context(), x.UserID, b.Telegram, b.Push); err != nil {
		return writeErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
