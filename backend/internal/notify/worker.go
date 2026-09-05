package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"retail-backend/internal/store"
)

// Worker отправляет due-уведомления через mock-каналы, переносит в историю.
// WEB считается доставленным сразу (входящие в кабинете).
func Worker(ctx context.Context, s *store.Store, interval time.Duration) {
	if s.PG == nil {
		slog.Error("notify worker: no PG, disabled")
		return
	}
	slog.Info("notify worker started", "interval", interval.String())
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("notify worker stopped")
			return
		case <-t.C:
			processBatch(ctx, s)
		}
	}
}

func processBatch(ctx context.Context, s *store.Store) {
	rows, err := s.PG.Query(ctx, `
		SELECT q.id, q.organization_id, q.notification_type_code, q.channel_code,
		       q.recipient_name, q.recipient_email, q.recipient_phone, q.recipient_user_id,
		       q.subject, q.body, q.template_data, q.entity_type, q.entity_id,
		       q.attempt_count, COALESCE(st.max_attempts, 3), COALESCE(st.fail_first_attempts, 0),
		       COALESCE(st.enabled, TRUE)
		FROM notification_queue q
		LEFT JOIN notify_settings st ON st.organization_id = q.organization_id
		WHERE q.status IN ('PENDING','RETRY') AND q.scheduled_at <= NOW()
		ORDER BY q.priority DESC, q.id LIMIT 20`)
	if err != nil {
		slog.Error("notify poll failed", "err", err)
		return
	}
	type job struct {
		id, org       int64
		ntype, ch     string
		name, email, phone *string
		uid           *int64
		subject, body *string
		tpl           []byte
		entity        *string
		entityID      *int64
		attempt, maxRet, failFirst int
		enabled       bool
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.org, &j.ntype, &j.ch, &j.name, &j.email, &j.phone,
			&j.uid, &j.subject, &j.body, &j.tpl, &j.entity, &j.entityID,
			&j.attempt, &j.maxRet, &j.failFirst, &j.enabled); err != nil {
			slog.Error("notify scan failed", "err", err)
			continue
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Error("notify rows failed", "err", err)
		return
	}

	for _, j := range jobs {
		if !j.enabled {
			continue
		}
		attempt := j.attempt + 1
		if attempt <= j.failFirst {
			_, err := s.PG.Exec(ctx, `
				UPDATE notification_queue SET attempt_count=$1, last_attempt_at=NOW(),
					status=CASE WHEN $1::int >= $2::int THEN 'FAILED' ELSE 'RETRY' END,
					error_message='mock: provider unavailable' WHERE id=$3`,
				attempt, j.maxRet, j.id)
			if err != nil {
				slog.Error("notify fail-mark failed", "id", j.id, "err", err)
			}
			continue
		}
		// Рендер тела: явное body или шаблон + template_data.
		body := str(j.body)
		subj := str(j.subject)
		if (body == "" || subj == "") && len(j.tpl) > 0 {
			var data map[string]interface{}
			_ = json.Unmarshal(j.tpl, &data)
			var tplSubj *string
			var tplBody string
			_ = s.PG.QueryRow(ctx, `
				SELECT subject, body_template FROM notification_template
				WHERE notification_type_code=$1 AND channel_code=$2 AND is_active LIMIT 1`,
				j.ntype, j.ch).Scan(&tplSubj, &tplBody)
			if tplBody != "" {
				if body == "" {
					body = Render(tplBody, data)
				}
				if subj == "" && tplSubj != nil && *tplSubj != "" {
					subj = Render(*tplSubj, data)
				}
			}
		}
		if body == "" {
			body = fmt.Sprintf("[%s] %s", j.ntype, str(j.subject))
		}
		provID := fmt.Sprintf("mock-%s-%d", j.ch, j.id)
		status := "SENT"
		if j.ch == "WEB" {
			status = "DELIVERED" // входящие сразу в кабинете
		}
		tx, err := s.PG.Begin(ctx)
		if err != nil {
			slog.Error("notify tx failed", "err", err)
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO notification_history(organization_id, notification_type_code, channel_code,
				recipient_name, recipient_email, recipient_phone, recipient_user_id,
				subject, body, entity_type, entity_id, status, provider_message_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			j.org, j.ntype, j.ch, j.name, j.email, j.phone, j.uid,
			subj, body, j.entity, j.entityID, status, provID)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM notification_queue WHERE id=$1`, j.id)
		}
		if err != nil {
			tx.Rollback(ctx)
			slog.Error("notify complete failed", "id", j.id, "err", err)
			continue
		}
		_ = tx.Commit(ctx)
		slog.Info("notify sent", "id", j.id, "channel", j.ch, "attempt", attempt)
	}
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
