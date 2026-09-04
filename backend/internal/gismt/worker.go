package gismt

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

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
	rows, err := s.PG.Query(ctx, `
		SELECT q.id, q.organization_id, q.marking_code_id, q.operation, q.receipt_id,
		       q.send_attempt, COALESCE(st.max_retries, 5), COALESCE(st.fail_first_attempts, 0),
		       COALESCE(st.auto_send_enabled, TRUE), m.code
		FROM gismt_queue q
		JOIN marking_code_pool m ON m.id = q.marking_code_id
		LEFT JOIN gismt_settings st ON st.organization_id = q.organization_id AND st.is_active
		WHERE q.status IN ('PENDING','RETRY')
		ORDER BY q.id LIMIT 20`)
	if err != nil {
		slog.Error("gismt poll failed", "err", err)
		return
	}
	type job struct {
		id, org, codeID int64
		op              string
		receipt         *int64
		attempt, maxRet, failFirst int
		auto            bool
		code            string
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.org, &j.codeID, &j.op, &j.receipt,
			&j.attempt, &j.maxRet, &j.failFirst, &j.auto, &j.code); err != nil {
			slog.Error("gismt scan failed", "err", err)
			continue
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Error("gismt rows failed", "err", err)
		return
	}

	for _, j := range jobs {
		if !j.auto {
			continue
		}
		attempt := j.attempt + 1
		if attempt <= j.failFirst {
			res, err := s.PG.Exec(ctx, `
				UPDATE gismt_queue SET send_attempt=$1, last_attempt_at=NOW(),
					status=CASE WHEN $1::int >= $2::int THEN 'FAILED' ELSE 'RETRY' END,
					error_message='mock: GIS MT unavailable', updated_at=NOW() WHERE id=$3`,
				attempt, j.maxRet, j.id)
			if err != nil {
				slog.Error("gismt fail-mark failed", "code", j.codeID, "err", err)
			} else {
				slog.Info("gismt mock-fail", "code", j.codeID, "attempt", attempt, "rows", res.RowsAffected())
			}
			continue
		}
		extID := Send(j.op, j.codeID)
		req, _ := json.Marshal(map[string]interface{}{"operation": j.op, "code": j.code, "receipt_id": j.receipt})
		resp, _ := json.Marshal(map[string]interface{}{"external_id": extID, "status": "ACCEPTED"})
		_, err := s.PG.Exec(ctx, `
			UPDATE gismt_queue SET send_attempt=$1, last_attempt_at=NOW(), status='COMPLETED',
				error_message=NULL, updated_at=NOW() WHERE id=$2`, attempt, j.id)
		if err != nil {
			slog.Error("gismt complete failed", "code", j.codeID, "err", err)
			continue
		}
		_, _ = s.PG.Exec(ctx, `
			INSERT INTO integration_log(organization_id, integration_type, direction, endpoint,
				request_data, response_data, external_id, document_id)
			VALUES($1,'GIS_MT','OUT','mock://gismt/documents',$2,$3,$4,$5)`,
			j.org, req, resp, extID, j.receipt)
		slog.Info("gismt completed", "code", j.codeID, "op", j.op, "attempt", attempt)
	}
}
