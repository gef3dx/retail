package ofd

import (
	"context"
	"log/slog"
	"time"

	"retail-backend/internal/provider"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

var reg = provider.DefaultRegistry()

// Worker раз в interval фискализирует PENDING/RETRY через активного провайдера:
// OFD_HTTP (если настроен) иначе OFD_EMULATOR. Без активного — честная ошибка.
func Worker(ctx context.Context, s *store.Store, interval time.Duration) {
	if s.PG == nil {
		slog.Error("ofd worker: no PG, disabled")
		return
	}
	slog.Info("ofd worker started", "interval", interval.String())
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("ofd worker stopped")
			return
		case <-t.C:
			processBatch(ctx, s)
		}
	}
}

func processBatch(ctx context.Context, s *store.Store) {
	repo := repository.OfdRepo{}
	intRepo := repository.IntegrationRepo{}
	for _, j := range repo.Poll(ctx, s.PG) {
		if !j.AutoSend {
			slog.Info("ofd skip: auto_send off", "receipt", j.ReceiptID)
			continue
		}
		attempt := j.Attempt + 1
		if attempt <= j.FailFirst {
			// Тестовый крючок: имитация недоступности ОФД.
			repo.FailMark(ctx, s.PG, j.ID, attempt, j.MaxRetries)
			slog.Info("ofd mock-fail", "receipt", j.ReceiptID, "attempt", attempt)
			continue
		}
		payload, err := repo.Payload(ctx, s.PG, j.ReceiptID)
		if err != nil {
			repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRetries, "receipt not found")
			continue
		}
		prov, creds := resolve(ctx, s, intRepo, j.OrgID)
		if prov == nil {
			repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRetries, "no active OFD provider: configure OFD_HTTP or enable emulator")
			slog.Info("ofd blocked: no provider", "receipt", j.ReceiptID)
			continue
		}
		r, err := prov.Fiscalize(ctx, creds, payload)
		if err != nil {
			repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRetries, err.Error())
			slog.Info("ofd fiscalize failed", "receipt", j.ReceiptID, "provider", prov.Code(), "err", err)
			continue
		}
		if err := repo.Complete(ctx, s.PG, j.ID, attempt, r.DocNumber, r.Sign, r.QRURL); err != nil {
			slog.Error("ofd complete failed", "receipt", j.ReceiptID, "err", err)
		} else {
			slog.Info("ofd completed", "receipt", j.ReceiptID, "provider", prov.Code(), "attempt", attempt)
		}
	}
}

// resolve выбирает провайдера: настроенный OFD_HTTP, иначе включённый эмулятор.
func resolve(ctx context.Context, s *store.Store, intRepo repository.IntegrationRepo, orgID int64) (KktProvider, map[string]string) {
	if creds, enabled, found := intRepo.Get(ctx, s.PG, orgID, "OFD_HTTP"); found && enabled {
		if p := reg.ByCode("OFD_HTTP"); p != nil && p.IsConfigured(creds) {
			return HTTPKkt{}, creds
		}
	}
	if _, enabled, found := intRepo.Get(ctx, s.PG, orgID, "OFD_EMULATOR"); !found || enabled {
		return Emulator{}, nil
	}
	return nil, nil
}
