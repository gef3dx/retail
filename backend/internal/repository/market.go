package repository

import (
	"context"
	"strings"

	"retail-backend/internal/model"
)

// OfferRepo — связки товаров с офферами.
type OfferRepo struct{}

func (OfferRepo) List(ctx context.Context, db DBTX, orgID int64, provider string) []model.OfferLink {
	q := `SELECT id, provider_code, product_id, offer_id FROM market_offer_link WHERE organization_id=$1`
	args := []interface{}{orgID}
	if provider != "" {
		args = append(args, provider)
		q += ` AND provider_code=$2`
	}
	q += ` ORDER BY id`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.OfferLink
	for rows.Next() {
		var l model.OfferLink
		_ = rows.Scan(&l.ID, &l.Provider, &l.ProductID, &l.OfferID)
		out = append(out, l)
	}
	if out == nil {
		out = []model.OfferLink{}
	}
	return out
}

func (OfferRepo) Create(ctx context.Context, db DBTX, orgID int64, provider string, productID int64, offerID string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO market_offer_link(organization_id, provider_code, product_id, offer_id)
		VALUES($1,$2,$3,$4) RETURNING id`, orgID, provider, productID, offerID).Scan(&id)
	return id, err
}

func (OfferRepo) Delete(ctx context.Context, db DBTX, orgID, id int64) bool {
	res, err := db.Exec(ctx, `DELETE FROM market_offer_link WHERE id=$1 AND organization_id=$2`, id, orgID)
	if err != nil || res.RowsAffected() == 0 {
		return false
	}
	return true
}

// ProductByOffer ищет товар по офферу, иначе по SKU.
func (OfferRepo) ProductByOffer(ctx context.Context, db DBTX, orgID int64, provider, offer string) (int64, float64, bool) {
	var pid int64
	var vat float64
	if err := db.QueryRow(ctx, `
		SELECT l.product_id, p.vat_rate FROM market_offer_link l
		JOIN catalog_product p ON p.id=l.product_id
		WHERE l.organization_id=$1 AND l.provider_code=$2 AND l.offer_id=$3`, orgID, provider, offer).
		Scan(&pid, &vat); err == nil {
		return pid, vat, true
	}
	if err := db.QueryRow(ctx, `
		SELECT id, vat_rate FROM catalog_product WHERE sku=$1 AND is_active`, offer).
		Scan(&pid, &vat); err == nil {
		return pid, vat, true
	}
	return 0, 0, false
}

// MarketOrderRepo — заказы маркетплейсов.
type MarketOrderRepo struct{}

func (MarketOrderRepo) Exists(ctx context.Context, db DBTX, orgID int64, provider, externalID string) bool {
	var n int
	_ = db.QueryRow(ctx, `
		SELECT COUNT(*) FROM marketplace_order
		WHERE organization_id=$1 AND provider_code=$2 AND external_order_id=$3`, orgID, provider, externalID).Scan(&n)
	return n > 0
}

func (MarketOrderRepo) Create(ctx context.Context, db DBTX, orgID int64, provider, externalID string,
	buyerName, buyerPhone string, total float64, mpStatus string, raw []byte,
	salesOrder *int64, status, errMsg string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO marketplace_order(organization_id, provider_code, external_order_id,
			buyer_name, buyer_phone, total_amount, marketplace_status, sales_order_id, raw_data, status, error_message)
		VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,NULLIF($7,''),$8,$9,$10,NULLIF($11,'')) RETURNING id`,
		orgID, provider, externalID, buyerName, buyerPhone, total, mpStatus, salesOrder, raw, status, errMsg).Scan(&id)
	return id, err
}

func (MarketOrderRepo) AddItem(ctx context.Context, db DBTX, orderID int64, productID *int64,
	offerID, name string, qty, price float64) {
	_, _ = db.Exec(ctx, `
		INSERT INTO marketplace_order_item(order_id, product_id, external_offer_id, product_name, quantity, price)
		VALUES($1,$2,NULLIF($3,''),$4,$5,$6)`, orderID, productID, offerID, name, qty, price)
}

func (MarketOrderRepo) List(ctx context.Context, db DBTX, orgID int64, provider string) []model.MarketOrder {
	q := `SELECT id, provider_code, external_order_id, buyer_name, total_amount, status,
		marketplace_status, sales_order_id, error_message FROM marketplace_order WHERE organization_id=$1`
	args := []interface{}{orgID}
	if provider != "" {
		args = append(args, provider)
		q += ` AND provider_code=$2`
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.MarketOrder
	for rows.Next() {
		var o model.MarketOrder
		_ = rows.Scan(&o.ID, &o.Provider, &o.ExternalID, &o.Buyer, &o.Total, &o.Status, &o.MpStatus, &o.SalesOrder, &o.Error)
		out = append(out, o)
	}
	if out == nil {
		out = []model.MarketOrder{}
	}
	return out
}

func (MarketOrderRepo) Items(ctx context.Context, db DBTX, orderID int64) []model.MarketOrderItem {
	rows, err := db.Query(ctx, `
		SELECT id, product_id, external_offer_id, product_name, quantity, price
		FROM marketplace_order_item WHERE order_id=$1 ORDER BY id`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.MarketOrderItem
	for rows.Next() {
		var it model.MarketOrderItem
		_ = rows.Scan(&it.ID, &it.ProductID, &it.OfferID, &it.Name, &it.Quantity, &it.Price)
		out = append(out, it)
	}
	if out == nil {
		out = []model.MarketOrderItem{}
	}
	return out
}

// BuyerCounterparty возвращает (или создает) контрагента-покупателя маркетплейса.
// INN короткий (VARCHAR(12)): MP-<первые 8 символов кода без префикса MARKET_.
func (MarketOrderRepo) BuyerCounterparty(ctx context.Context, db DBTX, orgID int64, provider string) (int64, error) {
	tag := strings.TrimPrefix(provider, "MARKET_")
	if len(tag) > 8 {
		tag = tag[:8]
	}
	inn := "MP-" + tag
	var id int64
	err := db.QueryRow(ctx, `SELECT id FROM counterparty WHERE organization_id=$1 AND inn=$2`, orgID, inn).Scan(&id)
	if err == nil {
		return id, nil
	}
	err = db.QueryRow(ctx, `
		INSERT INTO counterparty(organization_id, inn, full_name, is_buyer) VALUES($1,$2,$3,TRUE) RETURNING id`,
		orgID, inn, "Покупатели "+provider).Scan(&id)
	return id, err
}

// SyncLogRepo — журнал синхронизаций.
type SyncLogRepo struct{}

func (SyncLogRepo) Add(ctx context.Context, db DBTX, orgID int64, provider, direction, operation, status string,
	total, ok int, errMsg string) {
	_, _ = db.Exec(ctx, `
		INSERT INTO market_sync_log(organization_id, provider_code, direction, operation, status, items_total, items_ok, error_message)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''))`, orgID, provider, direction, operation, status, total, ok, errMsg)
}

func (SyncLogRepo) List(ctx context.Context, db DBTX, orgID int64, provider string) []model.SyncLog {
	q := `SELECT id, provider_code, direction, operation, status, items_total, items_ok, error_message, created_at::text
		FROM market_sync_log WHERE organization_id=$1`
	args := []interface{}{orgID}
	if provider != "" {
		args = append(args, provider)
		q += ` AND provider_code=$2`
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.SyncLog
	for rows.Next() {
		var l model.SyncLog
		_ = rows.Scan(&l.ID, &l.Provider, &l.Direction, &l.Operation, &l.Status, &l.Total, &l.OK, &l.Error, &l.At)
		out = append(out, l)
	}
	if out == nil {
		out = []model.SyncLog{}
	}
	return out
}
