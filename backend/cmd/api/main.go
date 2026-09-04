package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"retail-backend/internal/config"
	"retail-backend/internal/gismt"
	"retail-backend/internal/handler"
	"retail-backend/internal/ofd"
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
	catH := &handler.Catalog{Store: st}
	recH := &handler.Receipt{Store: st}
	markH := &handler.Marking{Store: st}

	// Фоновый воркер ОФД (mock). Интервал через env, по умолчанию 2с для живости кассы.
	ofdEvery := 2 * time.Second
	if v := os.Getenv("OFD_WORKER_INTERVAL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ofdEvery = time.Duration(n) * time.Second
		}
	}
	ofdCtx, stopOfd := context.WithCancel(ctx)
	defer stopOfd()
	go ofd.Worker(ofdCtx, st, ofdEvery)
	go gismt.Worker(ofdCtx, st, ofdEvery)

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

	// Каталог (этап 3)
	api.GET("/categories", catH.ListCategories, rbac.RequirePermission("product:read"))
	api.POST("/categories", catH.CreateCategory, rbac.RequirePermission("product:create"))
	api.GET("/brands", catH.ListBrands, rbac.RequirePermission("product:read"))
	api.POST("/brands", catH.CreateBrand, rbac.RequirePermission("product:create"))
	api.GET("/products", catH.ListProducts, rbac.RequirePermission("product:read"))
	api.GET("/products/by-code/:code", catH.ByCode, rbac.RequirePermission("product:read"))
	api.POST("/products", catH.CreateProduct, rbac.RequirePermission("product:create"))
	api.PATCH("/products/:id", catH.UpdateProduct, rbac.RequirePermission("product:update"))
	api.DELETE("/products/:id", catH.DeleteProduct, rbac.RequirePermission("product:delete"))
	api.GET("/products/:id/prices", catH.ListPrices, rbac.RequirePermission("product:read"))
	api.POST("/products/:id/prices", catH.AddPrice, rbac.RequirePermission("product:update"))
	api.GET("/price-types", catH.ListPriceTypes, rbac.RequirePermission("product:read"))

	// Касса (этап 4)
	api.GET("/registers", recH.ListRegisters, rbac.RequirePermission("organization:read"))
	api.POST("/registers", recH.CreateRegister, rbac.RequirePermission("organization:update"))
	api.POST("/shifts/open", recH.OpenShift, rbac.RequirePermission("receipt:create"))
	api.GET("/shifts/open", recH.OpenShiftForRegister, rbac.RequirePermission("receipt:read"))
	api.GET("/shifts/:id/report", recH.XReport, rbac.RequirePermission("receipt:read"))
	api.POST("/shifts/:id/close", recH.CloseShift, rbac.RequirePermission("receipt:create"))
	api.POST("/receipts/sell", recH.Sell, rbac.RequirePermission("receipt:create"))
	api.POST("/receipts/return", recH.Return, rbac.RequirePermission("receipt:return"))
	api.POST("/receipts/correction", recH.Correction, rbac.RequirePermission("receipt:create"))
	api.GET("/receipts", recH.ListReceipts, rbac.RequirePermission("receipt:read"))
	api.GET("/ofd-settings", recH.GetOfdSettings, rbac.RequirePermission("organization:read"))
	api.PATCH("/ofd-settings", recH.PatchOfdSettings, rbac.RequirePermission("organization:update"))

	// Маркировка (этап 5)
	api.POST("/marking/codes", markH.Register, rbac.RequirePermission("marking:manage"))
	api.GET("/marking/codes", markH.List, rbac.RequirePermission("marking:view"))
	api.GET("/marking/check/:code", markH.Check, rbac.RequirePermission("marking:view"))
	api.POST("/marking/write-off", markH.WriteOff, rbac.RequirePermission("marking:manage"))
	api.GET("/marking/queue", markH.Queue, rbac.RequirePermission("marking:view"))
	api.GET("/integrations/log", markH.Log, rbac.RequirePermission("marking:view"))

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
