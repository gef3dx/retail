package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"retail-backend/internal/config"
	"retail-backend/internal/handler"
	rbac "retail-backend/internal/middleware"
	"retail-backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL, cfg.RedisAddr)
	if err != nil {
		slog.Error("store init failed", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if st.PG != nil {
		handler.SeedAdmin(ctx, st, cfg.SeedAdminEmail, cfg.SeedAdminPass)
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.RequestID())

	handler.Health(e, st)

	authH := &handler.Auth{Store: st, JWTSecret: cfg.JWTSecret,
		AccessTTL: time.Duration(cfg.JWTAccessTTL) * time.Minute,
		RefreshTTL: time.Duration(cfg.JWTRefreshDays) * 24 * time.Hour}
	usersH := &handler.Users{Store: st}
	orgsH := &handler.Orgs{Store: st}

	// Публичное
	e.POST("/api/v1/auth/register", authH.Register)
	e.POST("/api/v1/auth/login", authH.Login)
	e.POST("/api/v1/auth/refresh", authH.Refresh)
	e.POST("/api/v1/auth/logout", authH.Logout)

	// Приватное
	api := e.Group("/api/v1", rbac.AuthJWT(cfg.JWTSecret, st))
	api.GET("/me", authH.Me)
	api.GET("/roles", orgsH.Roles)
	api.GET("/organizations", orgsH.List, rbac.RequirePermission("organization:read"))
	api.POST("/organizations", orgsH.Create, rbac.RequirePermission("organization:create"))
	api.GET("/users", usersH.List, rbac.RequirePermission("user:read"))
	api.POST("/users", usersH.Create, rbac.RequirePermission("user:create"))
	api.POST("/users/:id/roles", usersH.AssignRole, rbac.RequirePermission("user:role"))
	api.GET("/audit", orgsH.Audit, rbac.RequirePermission("user:read"))

	slog.Info("backend starting", "port", cfg.Port, "env", cfg.Env)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: e}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	slog.Info("backend stopped")
}
