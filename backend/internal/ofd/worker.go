package ofd

import (
	"context"
	"log/slog"
	"time"

	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// Worker раз в interval забирает PENDING/RETRY и "отправляет" через Mock.
// Останавливается по ctx.Done(). Легкий: один запрос за тик.
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
	jobs := repo.Poll(ctx, s.PG)
	for _, j := range jobs {
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
		r := Send(j.ReceiptID)
		if err := repo.Complete(ctx, s.PG, j.ID, attempt, r.DocNumber, r.Sign, r.QRURL); err != nil {
			slog.Error("ofd complete failed", "receipt", j.ReceiptID, "err", err)
		} else {
			slog.Info("ofd completed", "receipt", j.ReceiptID, "attempt", attempt)
		}
	}
}
