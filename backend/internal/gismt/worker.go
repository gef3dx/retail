package gismt

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"retail-backend/internal/provider"
	"retail-backend/internal/repository"

	"retail-backend/internal/store"
)

var reg = provider.DefaultRegistry()

// Worker раз в interval отправляет PENDING/RETRY через активного провайдера:
// GISMT_TRUEAPI (если настроен) иначе GISMT_EMULATOR. Без активного — честная
// ошибка. Результат пишет в integration_log. Останавливается по ctx.Done().
func Worker(ctx context.Context, s *store.Store, interval time.Duration) {
	if s.PG == nil {
		slog.Error("gismt worker: no PG, disabled")
		return
	}
	slog.Info("gismt worker started", "interval", interval.String())
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("gismt worker stopped")
			return
		case <-t.C:
			processBatch(ctx, s)
		}
	}
}

func processBatch(ctx context.Context, s *store.Store) {
	repo := repository.GismtRepo{}
	intRepo := repository.IntegrationRepo{}
	for _, j := range repo.Poll(ctx, s.PG) {
		if !j.AutoSend {
			continue
		}
		attempt := j.Attempt + 1
		if attempt <= j.FailFirst {
			repo.FailMark(ctx, s.PG, j.ID, attempt, j.MaxRet)
			slog.Info("gismt mock-fail", "code", j.CodeID, "attempt", attempt)
			continue
		}
		prov, creds := resolve(ctx, s, intRepo, j.OrgID)
		if prov == nil {
			repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRet, "no active GIS MT provider: configure GISMT_TRUEAPI or enable emulator")
			slog.Info("gismt blocked: no provider", "code", j.CodeID)
			continue
		}
		op := GismtOp{Operation: j.Op, Code: j.Code, CodeID: j.CodeID, ReceiptID: j.ReceiptID}
		extID, err := prov.SendOp(ctx, creds, op)
		if err != nil {
			repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRet, err.Error())
			slog.Info("gismt send failed", "code", j.CodeID, "provider", prov.Code(), "err", err)
			continue
		}
		req, _ := json.Marshal(map[string]interface{}{"operation": j.Op, "code": j.Code, "receipt_id": j.ReceiptID})
		resp, _ := json.Marshal(map[string]interface{}{"external_id": extID, "status": "ACCEPTED"})
		if err := repo.Complete(ctx, s.PG, j.ID, attempt); err != nil {
			slog.Error("gismt complete failed", "code", j.CodeID, "err", err)
			continue
		}
		repo.LogIntegration(ctx, s.PG, j.OrgID, endpointFor(prov.Code(), creds), req, resp, extID, j.ReceiptID)
		slog.Info("gismt completed", "code", j.CodeID, "op", j.Op, "provider", prov.Code(), "attempt", attempt)
	}
}

// resolve выбирает провайдера: настроенный GISMT_TRUEAPI, иначе включённый эмулятор.
func resolve(ctx context.Context, s *store.Store, intRepo repository.IntegrationRepo, orgID int64) (GismtProvider, map[string]string) {
	if creds, enabled, found := intRepo.Get(ctx, s.PG, orgID, "GISMT_TRUEAPI"); found && enabled {
		if p := reg.ByCode("GISMT_TRUEAPI"); p != nil && p.IsConfigured(creds) {
			return TrueAPI{}, creds
		}
	}
	if _, enabled, found := intRepo.Get(ctx, s.PG, orgID, "GISMT_EMULATOR"); !found || enabled {
		return Emulator{}, nil
	}
	return nil, nil
}

func endpointFor(code string, creds map[string]string) string {
	if code == "GISMT_TRUEAPI" {
		if base := creds["api_base"]; base != "" {
			return base + "/outgoing"
		}
		return "https://markirovka.crpt.ru/api/v4/true-api/outgoing"
	}
	return "emulator://gismt/documents"
}
