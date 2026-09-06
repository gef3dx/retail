package market

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ExtBuyer — покупатель из маркетплейса.
type ExtBuyer struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

// ExtItem — позиция внешнего заказа.
type ExtItem struct {
	OfferID string  `json:"offer_id"`
	Name    string  `json:"name"`
	Qty     float64 `json:"qty"`
	Price   float64 `json:"price"`
}

// ExtOrder — нормализованный внешний заказ.
type ExtOrder struct {
	ExternalID string          `json:"external_id"`
	Number     string          `json:"number"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	Buyer      ExtBuyer        `json:"buyer"`
	Items      []ExtItem       `json:"items"`
	Raw        json.RawMessage `json:"raw"`
}

// StockItem — остаток для выгрузки.
type StockItem struct {
	OfferID string  `json:"offer_id"`
	SKU     string  `json:"sku"`
	Qty     float64 `json:"qty"`
}

// MarketplaceProvider — клиент API маркетплейса.
type MarketplaceProvider interface {
	Code() string
	PullOrders(ctx context.Context, creds map[string]string) ([]ExtOrder, error)
	PushStocks(ctx context.Context, creds map[string]string, items []StockItem) error
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

func baseURL(creds map[string]string, def string) string {
	if creds["api_url"] != "" {
		return creds["api_url"]
	}
	return def
}

func doJSON(ctx context.Context, method, url string, headers map[string]string, payload interface{}) ([]byte, int, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("market http: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

func toFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	}
	return 0
}

func toStr(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ------------------------------------------------------------- Ozon

// Ozon — api-seller.ozon.ru: POST /v2/posting/fbo/list,
// POST /v2/products/stocks.
type Ozon struct{}

func (Ozon) Code() string { return "MARKET_OZON" }

func (Ozon) PullOrders(ctx context.Context, creds map[string]string) ([]ExtOrder, error) {
	base := baseURL(creds, "https://api-seller.ozon.ru")
	headers := map[string]string{"Client-Id": creds["client_id"], "Api-Key": creds["api_key"]}
	b, code, err := doJSON(ctx, "POST", base+"/v2/posting/fbo/list", headers, map[string]interface{}{
		"dir": "ASC", "filter": map[string]interface{}{}, "limit": 50, "offset": 0, "translit": true,
		"with": map[string]bool{"analytics_data": false, "financial_data": false},
	})
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("ozon: status %d", code)
	}
	var out struct {
		Result struct {
			Postings []map[string]interface{} `json:"postings"`
		} `json:"result"`
	}
	_ = json.Unmarshal(b, &out)
	var orders []ExtOrder
	for _, p := range out.Result.Postings {
		o := ExtOrder{
			ExternalID: fmt.Sprintf("%v", p["order_id"]),
			Number:     toStr(p["order_number"]),
			Status:     toStr(p["status"]),
		}
		if o.ExternalID == "" || o.ExternalID == "<nil>" {
			continue
		}
		if prods, ok := p["products"].([]interface{}); ok {
			for _, pi := range prods {
				if pm, ok := pi.(map[string]interface{}); ok {
					o.Items = append(o.Items, ExtItem{
						OfferID: toStr(pm["offer_id"]),
						Name:    toStr(pm["name"]),
						Qty:     toFloat(pm["quantity"]),
						Price:   toFloat(pm["price"]),
					})
				}
			}
		}
		raw, _ := json.Marshal(p)
		o.Raw = raw
		orders = append(orders, o)
	}
	return orders, nil
}

func (Ozon) PushStocks(ctx context.Context, creds map[string]string, items []StockItem) error {
	base := baseURL(creds, "https://api-seller.ozon.ru")
	headers := map[string]string{"Client-Id": creds["client_id"], "Api-Key": creds["api_key"]}
	var stocks []map[string]interface{}
	for _, it := range items {
		stocks = append(stocks, map[string]interface{}{"offer_id": it.OfferID, "stock": int(it.Qty)})
	}
	_, code, err := doJSON(ctx, "POST", base+"/v2/products/stocks", headers, map[string]interface{}{"stocks": stocks})
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("ozon: status %d", code)
	}
	return nil
}

// ------------------------------------------------------------- Wildberries

// Wildberries — marketplace-api.wildberries.ru: GET /api/v3/orders/new,
// PUT /api/v3/stocks/{warehouse}.
type Wildberries struct{}

func (Wildberries) Code() string { return "MARKET_WB" }

func (Wildberries) PullOrders(ctx context.Context, creds map[string]string) ([]ExtOrder, error) {
	base := baseURL(creds, "https://marketplace-api.wildberries.ru")
	headers := map[string]string{"Authorization": creds["api_key"]}
	b, code, err := doJSON(ctx, "GET", base+"/api/v3/orders/new", headers, nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("wb: status %d", code)
	}
	var out struct {
		Orders []map[string]interface{} `json:"orders"`
	}
	_ = json.Unmarshal(b, &out)
	var orders []ExtOrder
	for _, p := range out.Orders {
		id := firstNonEmpty3(toStr(p["id"]), toStr(p["orderUID"]), toStr(p["orderUid"]))
		if id == "" {
			continue
		}
		o := ExtOrder{ExternalID: id, Number: id, Status: toStr(p["status"])}
		for _, key := range []string{"items", "products", "orderLines"} {
			if arr, ok := p[key].([]interface{}); ok {
				for _, pi := range arr {
					if pm, ok := pi.(map[string]interface{}); ok {
						o.Items = append(o.Items, ExtItem{
							OfferID: firstNonEmpty3(toStr(pm["offerId"]), toStr(pm["sku"]), toStr(pm["offer_id"])),
							Name:    toStr(pm["name"]),
							Qty:     toFloat(pm["quantity"]),
							Price:   firstNonEmptyFloat(pm, "price"),
						})
					}
				}
			}
		}
		raw, _ := json.Marshal(p)
		o.Raw = raw
		orders = append(orders, o)
	}
	return orders, nil
}

func (Wildberries) PushStocks(ctx context.Context, creds map[string]string, items []StockItem) error {
	base := baseURL(creds, "https://marketplace-api.wildberries.ru")
	headers := map[string]string{"Authorization": creds["api_key"]}
	wh := creds["warehouse_id"]
	if wh == "" {
		wh = creds["warehouseId"]
	}
	if wh == "" {
		return fmt.Errorf("wb: warehouse_id required")
	}
	var stocks []map[string]interface{}
	for _, it := range items {
		sku := it.OfferID
		if sku == "" {
			sku = it.SKU
		}
		stocks = append(stocks, map[string]interface{}{"sku": sku, "amount": int(it.Qty)})
	}
	_, code, err := doJSON(ctx, "PUT", base+"/api/v3/stocks/"+wh, headers, stocks)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("wb: status %d", code)
	}
	return nil
}

// ------------------------------------------------------------- Yandex Market

// YandexMarket — api.partner.market.yandex.ru: заказы кампании, остатки офферов.
type YandexMarket struct{}

func (YandexMarket) Code() string { return "MARKET_YANDEX" }

func yandexHeaders(creds map[string]string) map[string]string {
	return map[string]string{"Api-Key": creds["api_key"]}
}

func (YandexMarket) PullOrders(ctx context.Context, creds map[string]string) ([]ExtOrder, error) {
	base := baseURL(creds, "https://api.partner.market.yandex.ru")
	camp := creds["campaign_id"]
	if camp == "" {
		return nil, fmt.Errorf("yandex: campaign_id required")
	}
	b, code, err := doJSON(ctx, "GET", base+"/campaigns/"+camp+"/orders", yandexHeaders(creds), nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("yandex: status %d", code)
	}
	var out struct {
		Orders []map[string]interface{} `json:"orders"`
	}
	_ = json.Unmarshal(b, &out)
	var orders []ExtOrder
	for _, p := range out.Orders {
		id := fmt.Sprintf("%v", p["id"])
		o := ExtOrder{ExternalID: id, Number: id, Status: toStr(p["status"])}
		if items, ok := p["items"].([]interface{}); ok {
			for _, pi := range items {
				if pm, ok := pi.(map[string]interface{}); ok {
					o.Items = append(o.Items, ExtItem{
						OfferID: firstNonEmpty(toStr(pm["offerId"]), toStr(pm["shopSku"])),
						Name:    toStr(pm["offerName"]),
						Qty:     toFloat(pm["count"]),
						Price:   firstNonEmptyFloat(pm, "price"),
					})
				}
			}
		}
		raw, _ := json.Marshal(p)
		o.Raw = raw
		orders = append(orders, o)
	}
	return orders, nil
}

func (YandexMarket) PushStocks(ctx context.Context, creds map[string]string, items []StockItem) error {
	base := baseURL(creds, "https://api.partner.market.yandex.ru")
	camp := creds["campaign_id"]
	if camp == "" {
		return fmt.Errorf("yandex: campaign_id required")
	}
	var skus []map[string]interface{}
	for _, it := range items {
		sku := it.OfferID
		if sku == "" {
			sku = it.SKU
		}
		skus = append(skus, map[string]interface{}{
			"sku": sku, "warehouseId": creds["warehouse_id"],
			"items": []map[string]interface{}{{"count": int(it.Qty)}},
		})
	}
	_, code, err := doJSON(ctx, "POST", base+"/campaigns/"+camp+"/offers/stocks",
		yandexHeaders(creds), map[string]interface{}{"skus": skus})
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("yandex: status %d", code)
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstNonEmptyFloat(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if f := toFloat(m[k]); f != 0 {
			return f
		}
	}
	return 0
}

func firstNonEmpty3(a, b, c string) string {
	if a != "" {
		return a
	}
	return firstNonEmpty(b, c)
}
