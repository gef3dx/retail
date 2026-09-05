package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"retail-backend/internal/provider"
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

// Worker отправляет due-уведомления через настроенных провайдеров.
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

// channelProviders — канал -> коды провайдеров по приоритету.
var channelProviders = map[string][]string{
	"EMAIL":    {"EMAIL_SMTP"},
	"TELEGRAM": {"TELEGRAM_BOT"},
	"SMS":      {"SMS_PROVIDER"},
	"PUSH":     {"PUSH_PROVIDER"},
	"WHATSAPP": {"WHATSAPP_GENERIC"},
}

var reg = provider.DefaultRegistry()

func senderFor(channel string) Sender {
	switch channel {
	case "TELEGRAM":
		return TelegramSender{}
	case "SMS":
		return GenericHTTPSender{DefaultToField: "to", DefaultTextField: "text"}
	case "PUSH":
		return GenericHTTPSender{DefaultToField: "token", DefaultTextField: "text"}
	case "WHATSAPP":
		return GenericHTTPSender{DefaultToField: "to", DefaultTextField: "text"}
	default:
		return nil
	}
}

// processBatch: WEB — внутренняя доставка; остальные каналы — через настроенного
// провайдера. Без активного провайдера — честная ошибка (RETRY/FAILED), без моков.
func processBatch(ctx context.Context, s *store.Store) {
	repo := repository.NotifyRepo{}
	intRepo := repository.IntegrationRepo{}
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
		if j.Channel == "WEB" {
			// Внутренняя доставка во входящие.
			tx, err := s.PG.Begin(ctx)
			if err != nil {
				slog.Error("notify tx failed", "err", err)
				continue
			}
			if err := repo.DrainToHistory(ctx, tx, j, subj, body, "DELIVERED", fmt.Sprintf("web-%d", j.ID)); err != nil {
				tx.Rollback(ctx)
				slog.Error("notify complete failed", "id", j.ID, "err", err)
				continue
			}
			_ = tx.Commit(ctx)
			slog.Info("notify sent", "id", j.ID, "channel", j.Channel, "attempt", attempt)
			continue
		}
		if j.Channel == "EMAIL" {
			creds, enabled, found := intRepo.Get(ctx, s.PG, j.OrgID, "EMAIL_SMTP")
			if !found || !enabled || !reg.ByCode("EMAIL_SMTP").IsConfigured(creds) {
				repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRet, "email provider not configured")
				slog.Info("notify blocked: no email provider", "id", j.ID)
				continue
			}
			provID, err := (SMTPSender{}).Send(ctx, creds, OutMessage{
				QueueID: j.ID, ToName: str(j.Name), ToEmail: str(j.Email),
				Subject: subj, Body: body, NotificationType: j.Type,
			})
			if err != nil {
				repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRet, err.Error())
				slog.Info("notify send failed", "id", j.ID, "err", err)
				continue
			}
			if err := drain(ctx, s, repo, j, subj, body, "SENT", provID); err != nil {
				slog.Error("notify complete failed", "id", j.ID, "err", err)
			}
			continue
		}
		// Остальные каналы — через реестр провайдеров.
		sent := false
		for _, code := range channelProviders[j.Channel] {
			p := reg.ByCode(code)
			if p == nil {
				continue
			}
			creds, enabled, found := intRepo.Get(ctx, s.PG, j.OrgID, code)
			if !found || !enabled || !p.IsConfigured(creds) {
				continue
			}
			sender := senderFor(j.Channel)
			if sender == nil {
				continue
			}
			provID, err := sender.Send(ctx, creds, OutMessage{
				QueueID: j.ID, ToName: str(j.Name), ToEmail: str(j.Email),
				ToPhone: str(j.Phone), ToTelegram: str(j.Telegram), ToPush: str(j.Push),
				Subject: subj, Body: body, NotificationType: j.Type,
			})
			if err != nil {
				repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRet, err.Error())
				slog.Info("notify send failed", "id", j.ID, "channel", j.Channel, "err", err)
			} else if derr := drain(ctx, s, repo, j, subj, body, "SENT", provID); derr != nil {
				slog.Error("notify complete failed", "id", j.ID, "err", derr)
			}
			sent = true
			break
		}
		if !sent {
			repo.MarkAttempt(ctx, s.PG, j.ID, attempt, j.MaxRet, "channel "+j.Channel+" not configured")
			slog.Info("notify blocked: no provider", "id", j.ID, "channel", j.Channel)
		}
	}
}

func drain(ctx context.Context, s *store.Store, repo repository.NotifyRepo, j repository.DueJob, subj, body, status, provID string) error {
	tx, err := s.PG.Begin(ctx)
	if err != nil {
		return err
	}
	if err := repo.DrainToHistory(ctx, tx, j, subj, body, status, provID); err != nil {
		tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("notify sent", "id", j.ID, "channel", j.Channel)
	return nil
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
