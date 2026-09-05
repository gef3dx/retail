package ofd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"retail-backend/internal/model"
)

// Доменные типы фискализации живут в model (без циклов импорта).
type (
	FiscalItem    = model.FiscalItem
	FiscalPayload = model.FiscalPayload
	FiscalResult  = model.FiscalResult
)

// KktProvider — провайдер фискализации.
type KktProvider interface {
	Code() string
	Fiscalize(ctx context.Context, creds map[string]string, p FiscalPayload) (FiscalResult, error)
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Emulator — встроенный эмулятор ККТ (только разработка).
// Детерминирован: один чек всегда даёт один ФД/ФП.
type Emulator struct{}

func (Emulator) Code() string { return "OFD_EMULATOR" }

func (Emulator) Fiscalize(_ context.Context, _ map[string]string, p FiscalPayload) (FiscalResult, error) {
	h := sha256.Sum256([]byte(fmt.Sprintf("emulator-ofd:%d", p.ReceiptID)))
	return FiscalResult{
		DocNumber: fmt.Sprintf("%d", 900000+p.ReceiptID),
		Sign:      fmt.Sprintf("%x", h)[:32],
		QRURL:     fmt.Sprintf("https://emulator.ofd/check?fd=%d&fp=%x", 900000+p.ReceiptID, h),
	}, nil
}

// HTTPKkt — универсальный HTTP-адаптер ОФД.
// POST {api_url} JSON {api_key, receipt} -> {fiscal_document_number, fiscal_sign, qr_url}.
type HTTPKkt struct{}

func (HTTPKkt) Code() string { return "OFD_HTTP" }

func (HTTPKkt) Fiscalize(ctx context.Context, creds map[string]string, p FiscalPayload) (FiscalResult, error) {
	url := creds["api_url"]
	if url == "" {
		return FiscalResult{}, fmt.Errorf("ofd http: api_url required")
	}
	body, _ := json.Marshal(map[string]interface{}{"api_key": creds["api_key"], "receipt": p})
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return FiscalResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return FiscalResult{}, fmt.Errorf("ofd http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FiscalResult{}, fmt.Errorf("ofd http: status %s", resp.Status)
	}
	var out struct {
		FiscalDocumentNumber string `json:"fiscal_document_number"`
		FiscalSign           string `json:"fiscal_sign"`
		QRURL                string `json:"qr_url"`
		Error                string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Error != "" {
		return FiscalResult{}, fmt.Errorf("ofd: %s", out.Error)
	}
	if out.FiscalDocumentNumber == "" || out.FiscalSign == "" {
		return FiscalResult{}, fmt.Errorf("ofd: incomplete response")
	}
	return FiscalResult{DocNumber: out.FiscalDocumentNumber, Sign: out.FiscalSign, QRURL: out.QRURL}, nil
}

// Result — фискальные данные (совместимость).
// Deprecated: используйте FiscalResult.
type Result = FiscalResult

// Send — совместимость со старым вызовом (эмулятор).
// Deprecated: используйте KktProvider.
func Send(receiptID int64) Result {
	r, _ := (Emulator{}).Fiscalize(context.Background(), nil, FiscalPayload{ReceiptID: receiptID})
	return r
}
