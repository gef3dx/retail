package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

var varRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// Render подставляет {{var}} из data.
func Render(tpl string, data map[string]interface{}) string {
	return varRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := varRe.FindStringSubmatch(m)[1]
		if v, ok := data[key]; ok && v != nil {
			switch t := v.(type) {
			case string:
				return t
			default:
				b, _ := json.Marshal(v)
				return string(b)
			}
		}
		return m
	})
}

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
	repo := repository.NotifyRepo{}
	for _, j := range repo.PollDue(ctx, s.PG) {
		if !j.Enabled {
			continue
		}
		attempt := j.Attempt + 1
		if attempt <= j.FailFirst {
			repo.FailMark(ctx, s.PG, j.ID, attempt, j.MaxRet)
			continue
		}
		// Рендер тела: явное body или шаблон + template_data.
		body := str(j.Body)
		subj := str(j.Subject)
		if (body == "" || subj == "") && len(j.TemplateData) > 0 {
			var data map[string]interface{}
			_ = json.Unmarshal(j.TemplateData, &data)
			tplSubj, tplBody := repo.TemplateFor(ctx, s.PG, j.Type, j.Channel)
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
			body = fmt.Sprintf("[%s] %s", j.Type, str(j.Subject))
		}
		provID := fmt.Sprintf("mock-%s-%d", j.Channel, j.ID)
		status := "SENT"
		if j.Channel == "WEB" {
			status = "DELIVERED" // входящие сразу в кабинете
		}
		tx, err := s.PG.Begin(ctx)
		if err != nil {
			slog.Error("notify tx failed", "err", err)
			continue
		}
		if err := repo.DrainToHistory(ctx, tx, j, subj, body, status, provID); err != nil {
			tx.Rollback(ctx)
			slog.Error("notify complete failed", "id", j.ID, "err", err)
			continue
		}
		_ = tx.Commit(ctx)
		slog.Info("notify sent", "id", j.ID, "channel", j.Channel, "attempt", attempt)
	}
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
