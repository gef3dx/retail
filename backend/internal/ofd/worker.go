package ofd

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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
	rows, err := s.PG.Query(ctx, `
		SELECT o.id, o.receipt_id, o.organization_id, o.send_attempt,
		       COALESCE(st.max_retries, 3), COALESCE(st.fail_first_attempts, 0),
		       COALESCE(st.auto_send_enabled, TRUE)
		FROM ofd_send_status o
		LEFT JOIN ofd_settings st ON st.organization_id = o.organization_id AND st.is_active
		WHERE o.status IN ('PENDING','RETRY')
		ORDER BY o.id LIMIT 20`)
	if err != nil {
		slog.Error("ofd poll failed", "err", err)
		return
	}
	type job struct {
		id, receipt, org int64
		attempt, maxRet, failFirst int
		auto bool
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.receipt, &j.org, &j.attempt, &j.maxRet, &j.failFirst, &j.auto); err != nil {
			slog.Error("ofd scan failed", "err", err)
			continue
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Error("ofd rows failed", "err", err)
		return
	}

	for _, j := range jobs {
		if !j.auto {
			slog.Info("ofd skip: auto_send off", "receipt", j.receipt)
			continue
		}
		attempt := j.attempt + 1
		if attempt <= j.failFirst {
			// Тестовый крючок: имитация недоступности ОФД.
			res, err := s.PG.Exec(ctx, `
				UPDATE ofd_send_status SET send_attempt=$1, last_attempt_at=NOW(),
					status=CASE WHEN $1::int >= $2::int THEN 'FAILED' ELSE 'RETRY' END,
					error_message='mock: OFD unavailable', updated_at=NOW() WHERE id=$3`,
				attempt, j.maxRet, j.id)
			if err != nil {
				slog.Error("ofd fail-mark failed", "receipt", j.receipt, "err", err)
			} else {
				slog.Info("ofd mock-fail", "receipt", j.receipt, "attempt", attempt, "rows", res.RowsAffected())
			}
			continue
		}
		r := Send(j.receipt)
		err := s.Tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `
				UPDATE ofd_send_status SET send_attempt=$1, last_attempt_at=NOW(), status='COMPLETED',
					fiscal_document_number=$2, fiscal_sign=$3, qr_code_url=$4,
					error_message=NULL, updated_at=NOW() WHERE id=$5`,
				attempt, r.DocNumber, r.Sign, r.QRURL, j.id); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			slog.Error("ofd complete failed", "receipt", j.receipt, "err", err)
		} else {
			slog.Info("ofd completed", "receipt", j.receipt, "attempt", attempt)
		}
	}
}
