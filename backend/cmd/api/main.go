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
	"retail-backend/internal/booking"
	"retail-backend/internal/config"
	"retail-backend/internal/gismt"
	"retail-backend/internal/handler"
	rbac "retail-backend/internal/middleware"
	"retail-backend/internal/notify"
	"retail-backend/internal/ofd"
	"retail-backend/internal/provider"
	"retail-backend/internal/repository"
	"retail-backend/internal/service"
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
		service.SeedAdmin(ctx, st, cfg.SeedAdminEmail, cfg.SeedAdminPass)
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.RequestID())

	handler.Health(e, st)

	authH := &handler.Auth{Svc: &service.AuthService{
		Store: st, Users: repository.UserRepo{}, Orgs: repository.OrgRepo{}, Audit: repository.AuditRepo{},
		JWTSecret:  cfg.JWTSecret,
		AccessTTL:  time.Duration(cfg.JWTAccessTTL) * time.Minute,
		RefreshTTL: time.Duration(cfg.JWTRefreshDays) * 24 * time.Hour,
	}}
	usersH := &handler.Users{Svc: &service.UserService{Store: st, Users: repository.UserRepo{}, Audit: repository.AuditRepo{}}}
	orgsH := &handler.Orgs{Svc: &service.OrgService{Store: st, Orgs: repository.OrgRepo{}, Audit: repository.AuditRepo{}}}
	catH := &handler.Catalog{Svc: &service.CatalogService{
		Store: st, Categories: repository.CategoryRepo{}, Brands: repository.BrandRepo{},
		Products: repository.ProductRepo{}, Audit: repository.AuditRepo{},
	}}
	recH := &handler.Receipt{Svc: &service.ReceiptService{
		Store: st, Registers: repository.RegisterRepo{}, Shifts: repository.ShiftRepo{},
		Receipts: repository.ReceiptRepo{}, Products: repository.ProductRepo{},
		Balances: repository.BalanceRepo{}, Marking: repository.MarkingRepo{},
		Notify: repository.NotifyRepo{}, Ofd: repository.OfdRepo{}, Audit: repository.AuditRepo{},
	}}
	markH := &handler.Marking{Svc: &service.MarkingService{
		Store: st, Codes: repository.CodesRepo{}, Audit: repository.AuditRepo{},
	}}
	notifyH := &handler.Notify{Svc: &service.NotifyService{Store: st, Queue: repository.NotifyRepo{}}}
	stockH := &handler.Stock{Svc: &service.StockService{
		Store: st, Counterparties: repository.CounterpartyRepo{}, Warehouses: repository.WarehouseRepo{},
		Docs: repository.StockDocRepo{}, Orders: repository.OrderRepo{}, Shipments: repository.ShipmentRepo{},
		Products: repository.ProductRepo{}, Balance: repository.BalanceRepo{}, Notify: repository.NotifyRepo{},
	}}
	intH := &handler.Integrations{Svc: &service.IntegrationService{
		Store: st, Regs: repository.IntegrationRepo{}, Reg: provider.DefaultRegistry(),
	}}
	bookH := &handler.Booking{Svc: &service.BookingService{
		Store: st, Resources: repository.ResourceRepo{}, Bookings: repository.BookingRepo{}, Notify: repository.NotifyRepo{},
	}}

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
	go notify.Worker(ofdCtx, st, ofdEvery)
	remindEvery := 60 * time.Second
	if v := os.Getenv("BOOKING_REMIND_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			remindEvery = time.Duration(n) * time.Second
		}
	}
	go booking.Worker(ofdCtx, st, remindEvery)

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
	api.PATCH("/registers/:id", recH.PatchRegister, rbac.RequirePermission("organization:update"))
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

	// Склад и заказы (этап 6)
	api.GET("/counterparties", stockH.ListCounterparties, rbac.RequirePermission("document:read"))
	api.POST("/counterparties", stockH.CreateCounterparty, rbac.RequirePermission("document:create"))
	api.GET("/warehouses", stockH.ListWarehouses, rbac.RequirePermission("document:read"))
	api.POST("/warehouses", stockH.CreateWarehouse, rbac.RequirePermission("organization:update"))
	api.GET("/stock/balances", stockH.Balances, rbac.RequirePermission("document:read"))
	api.POST("/stock/receipts", stockH.CreateReceiptDoc, rbac.RequirePermission("document:create"))
	api.GET("/stock/receipts", stockH.ListReceiptDocs, rbac.RequirePermission("document:read"))
	api.POST("/stock/receipts/:id/post", stockH.PostReceiptDoc, rbac.RequirePermission("document:post"))
	api.POST("/orders", stockH.CreateOrder, rbac.RequirePermission("document:create"))
	api.GET("/orders", stockH.ListOrders, rbac.RequirePermission("document:read"))
	api.GET("/orders/:id", stockH.GetOrder, rbac.RequirePermission("document:read"))
	api.POST("/orders/:id/confirm", stockH.ConfirmOrder, rbac.RequirePermission("document:post"))
	api.POST("/orders/:id/cancel", stockH.CancelOrder, rbac.RequirePermission("document:update"))
	api.POST("/shipments", stockH.CreateShipment, rbac.RequirePermission("document:post"))
	api.GET("/shipments", stockH.ListShipments, rbac.RequirePermission("document:read"))

	// Услуги и бронирование (этап 8)
	api.GET("/resources", bookH.ListResources, rbac.RequirePermission("document:read"))
	api.POST("/resources", bookH.CreateResource, rbac.RequirePermission("document:create"))
	api.GET("/resources/:id/schedule", bookH.GetSchedule, rbac.RequirePermission("document:read"))
	api.PUT("/resources/:id/schedule", bookH.PutSchedule, rbac.RequirePermission("document:update"))
	api.POST("/resources/:id/exceptions", bookH.AddException, rbac.RequirePermission("document:update"))
	api.GET("/resources/:id/slots", bookH.Slots, rbac.RequirePermission("document:read"))
	api.POST("/bookings", bookH.Create, rbac.RequirePermission("document:create"))
	api.GET("/bookings", bookH.List, rbac.RequirePermission("document:read"))
	api.GET("/bookings/:id", bookH.Get, rbac.RequirePermission("document:read"))
	api.POST("/bookings/:id/status", bookH.SetStatus, rbac.RequirePermission("document:update"))
	api.POST("/bookings/:id/link-receipt", bookH.LinkReceipt, rbac.RequirePermission("document:update"))
	api.POST("/products/:id/resources", bookH.LinkProductResource, rbac.RequirePermission("product:update"))
	api.GET("/products/:id/resources", bookH.ListProductResources, rbac.RequirePermission("product:read"))

	// Интеграции: провайдеры и ключи (этап 10)
	api.GET("/integrations", intH.List, rbac.RequirePermission("organization:read"))
	api.PUT("/integrations/:code", intH.Save, rbac.RequirePermission("organization:update"))
	api.DELETE("/integrations/:code", intH.Clear, rbac.RequirePermission("organization:update"))
	api.POST("/integrations/:code/test", intH.Test, rbac.RequirePermission("organization:read"))

	// Уведомления (этап 7)
	api.GET("/notify/inbox", notifyH.Inbox)
	api.POST("/notify/inbox/:id/viewed", notifyH.MarkViewed)
	api.GET("/notify/queue", notifyH.Queue, rbac.RequirePermission("report:view"))
	api.GET("/notify/templates", notifyH.Templates, rbac.RequirePermission("report:view"))
	api.POST("/notify/templates", notifyH.UpsertTemplate, rbac.RequirePermission("organization:update"))
	api.GET("/notify/preferences", notifyH.Preferences)
	api.PUT("/notify/preferences", notifyH.SetPreference)
	api.POST("/notify/send", notifyH.Send, rbac.RequirePermission("document:create"))
	api.GET("/notify/settings", notifyH.GetSettings, rbac.RequirePermission("organization:read"))
	api.PATCH("/notify/settings", notifyH.PatchSettings, rbac.RequirePermission("organization:update"))

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
