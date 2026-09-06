package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/market"
	"retail-backend/internal/model"
	"retail-backend/internal/provider"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// MarketService — маркетплейсы: офферы, заказы, остатки.
type MarketService struct {
	Store    *store.Store
	Reg      *provider.Registry
	IntRepo  repository.IntegrationRepo
	Offers   repository.OfferRepo
	Orders   repository.MarketOrderRepo
	Sync     repository.SyncLogRepo
	Products repository.ProductRepo
}

var marketProviders = map[string]market.MarketplaceProvider{
	"MARKET_OZON":   market.Ozon{},
	"MARKET_WB":     market.Wildberries{},
	"MARKET_YANDEX": market.YandexMarket{},
}

// resolve возвращает активного провайдера с credentials.
func (s *MarketService) resolve(ctx context.Context, orgID int64, code string) (market.MarketplaceProvider, map[string]string, error) {
	p, ok := marketProviders[code]
	if !ok {
		return nil, nil, NotFound("unknown provider")
	}
	creds, enabled, found := s.IntRepo.Get(ctx, s.Store.PG, orgID, code)
	if !found || !enabled {
		return nil, nil, Conflict("provider not configured: save keys in integrations")
	}
	if rp := s.Reg.ByCode(code); rp == nil || !rp.IsConfigured(creds) {
		return nil, nil, Conflict("provider not configured: save keys in integrations")
	}
	return p, creds, nil
}

func (s *MarketService) Providers(ctx context.Context, orgID int64) []map[string]interface{} {
	statuses := s.IntRepo.Statuses(ctx, s.Store.PG, s.Reg, orgID)
	var out []map[string]interface{}
	for _, st := range statuses {
		if _, ok := marketProviders[st.Code]; !ok {
			continue
		}
		out = append(out, map[string]interface{}{
			"code": st.Code, "name": st.Name, "status": st.Status,
			"enabled": st.Enabled, "missing": st.Missing,
		})
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	return out
}

// --- Offer links ---

type CreateOfferInput struct {
	OrgID     int64  `json:"org_id"`
	Provider  string `json:"provider_code"`
	ProductID int64  `json:"product_id"`
	OfferID   string `json:"offer_id"`
}

func (s *MarketService) ListOffers(ctx context.Context, orgID int64, provider string) []model.OfferLink {
	return s.Offers.List(ctx, s.Store.PG, orgID, provider)
}

func (s *MarketService) CreateOffer(ctx context.Context, in CreateOfferInput) (int64, error) {
	if in.OrgID == 0 || in.Provider == "" || in.ProductID == 0 || in.OfferID == "" {
		return 0, BadRequest("org_id/provider_code/product_id/offer_id required")
	}
	if _, ok := marketProviders[in.Provider]; !ok {
		return 0, BadRequest("unknown provider")
	}
	id, err := s.Offers.Create(ctx, s.Store.PG, in.OrgID, in.Provider, in.ProductID, in.OfferID)
	if err != nil {
		return 0, Conflict("duplicate offer link")
	}
	return id, nil
}

func (s *MarketService) DeleteOffer(ctx context.Context, orgID, id int64) error {
	if !s.Offers.Delete(ctx, s.Store.PG, orgID, id) {
		return NotFound("no link")
	}
	return nil
}

// --- Pull orders ---

func (s *MarketService) PullOrders(ctx context.Context, orgID int64, providerCode string, warehouseID, userID int64) (map[string]int, error) {
	if warehouseID == 0 {
		return nil, BadRequest("warehouse_id required")
	}
	p, creds, err := s.resolve(ctx, orgID, providerCode)
	if err != nil {
		return nil, err
	}
	ext, err := p.PullOrders(ctx, creds)
	if err != nil {
		s.Sync.Add(ctx, s.Store.PG, orgID, providerCode, "IN", "pull-orders", "FAILED", 0, 0, err.Error())
		return nil, Conflict("pull failed: " + err.Error())
	}
	matched, skipped := 0, 0
	var firstErr string
	for _, o := range ext {
		if s.Orders.Exists(ctx, s.Store.PG, orgID, providerCode, o.ExternalID) {
			continue
		}
		ok, soID, note := s.importOrder(ctx, orgID, providerCode, warehouseID, userID, o)
		if ok {
			matched++
			_ = soID
		} else {
			skipped++
			if firstErr == "" {
				firstErr = note
			}
		}
	}
	status := "OK"
	if skipped > 0 && matched == 0 {
		status = "FAILED"
	} else if skipped > 0 {
		status = "PARTIAL"
	}
	s.Sync.Add(ctx, s.Store.PG, orgID, providerCode, "IN", "pull-orders", status, len(ext), matched, firstErr)
	return map[string]int{"orders": len(ext), "matched": matched, "skipped": skipped}, nil
}

// importOrder сопоставляет позиции и создает sales_order. Возвращает (ok, salesOrderID, note).
func (s *MarketService) importOrder(ctx context.Context, orgID int64, providerCode string, warehouseID, userID int64, o market.ExtOrder) (bool, int64, string) {
	type line struct {
		pid   int64
		name  string
		offer string
		qty   float64
		price float64
		vat   float64
	}
	var lines []line
	for _, it := range o.Items {
		if it.Qty <= 0 {
			continue
		}
		pid, vat, ok := s.Offers.ProductByOffer(ctx, s.Store.PG, orgID, providerCode, it.OfferID)
		if !ok {
			return s.storeSkipped(ctx, orgID, providerCode, o, fmt.Sprintf("no product for offer %q", it.OfferID)), 0, "unmatched offer"
		}
		lines = append(lines, line{pid: pid, name: it.Name, offer: it.OfferID, qty: it.Qty, price: it.Price, vat: vat})
	}
	if len(lines) == 0 {
		return s.storeSkipped(ctx, orgID, providerCode, o, "empty items"), 0, "empty items"
	}
	var soID int64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		buyer, err := s.Orders.BuyerCounterparty(ctx, tx, orgID, providerCode)
		if err != nil {
			return err
		}
		total := 0.0
		for _, l := range lines {
			total += model.Round2(l.price * l.qty)
		}
		number := "MP-" + providerCode + "-" + o.ExternalID
		if err := tx.QueryRow(ctx, `
			INSERT INTO sales_order(organization_id, buyer_id, warehouse_id, order_number, order_type,
				total_amount, manager_id)
			VALUES($1,$2,$3,$4,'ONLINE',$5,$6) RETURNING id`,
			orgID, buyer, warehouseID, number, model.Round2(total), userID).Scan(&soID); err != nil {
			return err
		}
		for _, l := range lines {
			if _, err := tx.Exec(ctx, `
				INSERT INTO sales_order_line(order_id, product_id, quantity, price, vat_rate)
				VALUES($1,$2,$3,$4,$5)`, soID, l.pid, l.qty, l.price, l.vat); err != nil {
				return err
			}
		}
		mid, err := s.Orders.Create(ctx, tx, orgID, providerCode, o.ExternalID,
			o.Buyer.Name, o.Buyer.Phone, total, o.Status, o.Raw, &soID, "MATCHED", "")
		if err != nil {
			return err
		}
		for _, l := range lines {
			s.Orders.AddItem(ctx, tx, mid, &l.pid, l.offer, nz(l.name, o.ExternalID), l.qty, l.price)
		}
		return nil
	})
	if err != nil {
		return s.storeSkipped(ctx, orgID, providerCode, o, "create failed"), 0, "create failed"
	}
	return true, soID, ""
}

func nz(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func (s *MarketService) storeSkipped(ctx context.Context, orgID int64, providerCode string, o market.ExtOrder, note string) bool {
	total := 0.0
	for _, it := range o.Items {
		total += model.Round2(it.Price * it.Qty)
	}
	mid, _ := s.Orders.Create(ctx, s.Store.PG, orgID, providerCode, o.ExternalID,
		o.Buyer.Name, o.Buyer.Phone, model.Round2(total), o.Status, o.Raw, nil, "SKIPPED", note)
	for _, it := range o.Items {
		s.Orders.AddItem(ctx, s.Store.PG, mid, nil, it.OfferID, nz(it.Name, o.ExternalID), it.Qty, it.Price)
	}
	return false
}

// --- Push stocks ---

func (s *MarketService) PushStocks(ctx context.Context, orgID int64, providerCode string, warehouseID int64) (map[string]int, error) {
	p, creds, err := s.resolve(ctx, orgID, providerCode)
	if err != nil {
		return nil, err
	}
	links := s.Offers.List(ctx, s.Store.PG, orgID, providerCode)
	if len(links) == 0 {
		return nil, BadRequest("no offer links")
	}
	var items []market.StockItem
	for _, l := range links {
		var sku string
		var avail float64
		_ = s.Store.PG.QueryRow(ctx, `SELECT sku FROM catalog_product WHERE id=$1`, l.ProductID).Scan(&sku)
		if warehouseID != 0 {
			_ = s.Store.PG.QueryRow(ctx, `
				SELECT COALESCE(quantity,0)-COALESCE(reserved_quantity,0) FROM warehouse_balance
				WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, l.ProductID).Scan(&avail)
		} else {
			_ = s.Store.PG.QueryRow(ctx, `
				SELECT COALESCE(SUM(quantity-reserved_quantity),0) FROM warehouse_balance
				WHERE product_id=$1`, l.ProductID).Scan(&avail)
		}
		if avail < 0 {
			avail = 0
		}
		items = append(items, market.StockItem{OfferID: l.OfferID, SKU: sku, Qty: avail})
	}
	if err := p.PushStocks(ctx, creds, items); err != nil {
		s.Sync.Add(ctx, s.Store.PG, orgID, providerCode, "OUT", "push-stocks", "FAILED", len(items), 0, err.Error())
		return nil, Conflict("push failed: " + err.Error())
	}
	s.Sync.Add(ctx, s.Store.PG, orgID, providerCode, "OUT", "push-stocks", "OK", len(items), len(items), "")
	return map[string]int{"items": len(items)}, nil
}

// --- Reads ---

func (s *MarketService) ListOrders(ctx context.Context, orgID int64, provider string) []model.MarketOrder {
	orders := s.Orders.List(ctx, s.Store.PG, orgID, provider)
	for i := range orders {
		orders[i].Items = s.Orders.Items(ctx, s.Store.PG, orders[i].ID)
	}
	return orders
}

func (s *MarketService) ListSyncLog(ctx context.Context, orgID int64, provider string) []model.SyncLog {
	return s.Sync.List(ctx, s.Store.PG, orgID, provider)
}
