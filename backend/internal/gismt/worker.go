package gismt

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"retail-backend/internal/repository"

	"retail-backend/internal/store"
)

// Worker раз в interval отправляет PENDING/RETRY операции через Mock,
// пишет результат в integration_log. Останавливается по ctx.Done().
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
		extID := Send(j.Op, j.CodeID)
		req, _ := json.Marshal(map[string]interface{}{"operation": j.Op, "code": j.Code, "receipt_id": j.ReceiptID})
		resp, _ := json.Marshal(map[string]interface{}{"external_id": extID, "status": "ACCEPTED"})
		if err := repo.Complete(ctx, s.PG, j.ID, attempt); err != nil {
			slog.Error("gismt complete failed", "code", j.CodeID, "err", err)
			continue
		}
		repo.LogIntegration(ctx, s.PG, j.OrgID, "mock://gismt/documents", req, resp, extID, j.ReceiptID)
		slog.Info("gismt completed", "code", j.CodeID, "op", j.Op, "attempt", attempt)
	}
}
