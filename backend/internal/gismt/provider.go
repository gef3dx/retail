package gismt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GismtOp — операция с кодом маркировки для отправки провайдеру.
type GismtOp struct {
	Operation string `json:"operation"`
	Code      string `json:"code"`
	CodeID    int64  `json:"code_id"`
	ReceiptID *int64 `json:"receipt_id,omitempty"`
}

// GismtProvider — провайдер ГИС МТ.
type GismtProvider interface {
	Code() string
	SendOp(ctx context.Context, creds map[string]string, op GismtOp) (string, error)
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Emulator — встроенный эмулятор ГИС МТ (только разработка).
// Детерминирован: одна операция всегда даёт один external id.
type Emulator struct{}

func (Emulator) Code() string { return "GISMT_EMULATOR" }

func (Emulator) SendOp(_ context.Context, _ map[string]string, op GismtOp) (string, error) {
	h := sha256.Sum256([]byte(fmt.Sprintf("emulator-gismt:%s:%d", op.Operation, op.CodeID)))
	return fmt.Sprintf("GISMT-%x", h)[:20], nil
}

// TrueAPI — клиент API Честного знака (True API).
// POST {api_base}/outgoing с Bearer-токеном ОИС:
// {operation, code, receipt_id} -> {document_id | external_id}.
// Маппинг операций на типы документов True API настраивается шлюзом;
// без токена провайдер неактивен (см. реестр этапа 10).
type TrueAPI struct{}

func (TrueAPI) Code() string { return "GISMT_TRUEAPI" }

func (TrueAPI) SendOp(ctx context.Context, creds map[string]string, op GismtOp) (string, error) {
	base := creds["api_base"]
	if base == "" {
		base = "https://markirovka.crpt.ru/api/v4/true-api"
	}
	token := creds["token"]
	if token == "" {
		return "", fmt.Errorf("gismt trueapi: token required")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"operation":  op.Operation,
		"code":       op.Code,
		"receipt_id": op.ReceiptID,
		"oms_id":     creds["oms_id"],
	})
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/outgoing", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gismt trueapi: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gismt trueapi: status %s", resp.Status)
	}
	var out struct {
		DocumentID string `json:"document_id"`
		ExternalID string `json:"external_id"`
		Error      string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Error != "" {
		return "", fmt.Errorf("gismt: %s", out.Error)
	}
	if id := firstNonEmpty(out.DocumentID, out.ExternalID); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("gismt: incomplete response")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Send — совместимость со старым вызовом (эмулятор).
// Deprecated: используйте GismtProvider.
func Send(operation string, codeID int64) string {
	id, _ := (Emulator{}).SendOp(context.Background(), nil, GismtOp{Operation: operation, CodeID: codeID})
	return id
}
