package service

import (
	"context"
	"log/slog"

	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// NotifyService — постановка, чтение, шаблоны, предпочтения, настройки.
type NotifyService struct {
	Store *store.Store
	Queue repository.NotifyRepo
}

// Enqueue ставит уведомление в очередь (своя транзакция).
func (s *NotifyService) Enqueue(ctx context.Context, orgID int64, typeCode string,
	channels []string, r model.Recipient, subject, body string,
	data map[string]interface{}, entity string, entityID *int64, priority int) {
	tx, txErr := s.Store.PG.Begin(ctx)
	if txErr != nil {
		slog.Error("notify enqueue begin failed", "err", txErr)
		return
	}
	defer tx.Rollback(ctx)
	s.Queue.EnqueueTx(ctx, tx, orgID, typeCode, channels, r, subject, body, data, entity, entityID, priority)
	_ = tx.Commit(ctx)
}

// EnqueueUser — уведомление пользователю (контакты из users).
func (s *NotifyService) EnqueueUser(ctx context.Context, orgID int64, typeCode string,
	channels []string, uid int64, data map[string]interface{}, entity string, entityID *int64) {
	s.Enqueue(ctx, orgID, typeCode, channels, s.Queue.RecipientOf(ctx, s.Store.PG, uid), "", "", data, entity, entityID, 5)
}

func (s *NotifyService) Inbox(ctx context.Context, userID int64) []model.InboxItem {
	return s.Queue.Inbox(ctx, s.Store.PG, userID)
}

func (s *NotifyService) MarkViewed(ctx context.Context, id, userID int64) error {
	if !s.Queue.MarkViewed(ctx, s.Store.PG, id, userID) {
		return NotFound("not found")
	}
	return nil
}

func (s *NotifyService) QueueList(ctx context.Context, status string) []model.QueuedItem {
	return s.Queue.QueueList(ctx, s.Store.PG, status)
}

func (s *NotifyService) Templates(ctx context.Context) []model.Template {
	return s.Queue.Templates(ctx, s.Store.PG)
}

type UpsertTemplateInput struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *NotifyService) UpsertTemplate(ctx context.Context, in UpsertTemplateInput) error {
	if in.Type == "" || in.Channel == "" || in.Body == "" {
		return BadRequest("type/channel/body required")
	}
	if err := s.Queue.UpsertTemplate(ctx, s.Store.PG, in.Type, in.Channel, in.Name, in.Subject, in.Body); err != nil {
		return BadRequest("upsert failed (bad type/channel?)")
	}
	return nil
}

func (s *NotifyService) Preferences(ctx context.Context, userID int64) []model.Preference {
	return s.Queue.Preferences(ctx, s.Store.PG, userID)
}

func (s *NotifyService) SetPreference(ctx context.Context, userID int64, typ, channel string, enabled bool) error {
	if typ == "" || channel == "" {
		return BadRequest("type/channel required")
	}
	if err := s.Queue.SetPreference(ctx, s.Store.PG, userID, typ, channel, enabled); err != nil {
		return BadRequest("save failed")
	}
	return nil
}

type SendInput struct {
	OrgID    int64                  `json:"org_id"`
	Type     string                 `json:"type"`
	Channels []string               `json:"channels"`
	UserID   int64                  `json:"user_id"`
	Name     string                 `json:"name"`
	Email    string                 `json:"email"`
	Phone    string                 `json:"phone"`
	Subject  string                 `json:"subject"`
	Body     string                 `json:"body"`
	Data     map[string]interface{} `json:"data"`
	Entity   string                 `json:"entity"`
	EntityID *int64                 `json:"entity_id"`
}

func (s *NotifyService) Send(ctx context.Context, in SendInput) error {
	if in.OrgID == 0 || in.Type == "" || len(in.Channels) == 0 {
		return BadRequest("org_id/type/channels required")
	}
	r := model.Recipient{UserID: in.UserID, Name: in.Name, Email: in.Email, Phone: in.Phone}
	if r.UserID != 0 {
		// Дотягиваем пустые контакты из профиля.
		full := s.Queue.RecipientOf(ctx, s.Store.PG, r.UserID)
		if r.Name == "" {
			r.Name = full.Name
		}
		if r.Email == "" {
			r.Email = full.Email
		}
		if r.Phone == "" {
			r.Phone = full.Phone
		}
		if r.Telegram == "" {
			r.Telegram = full.Telegram
		}
		if r.Push == "" {
			r.Push = full.Push
		}
	}
	s.Enqueue(ctx, in.OrgID, in.Type, in.Channels, r,
		in.Subject, in.Body, in.Data, in.Entity, in.EntityID, 5)
	return nil
}

func (s *NotifyService) GetSettings(ctx context.Context, orgID int64) (model.NotifySettings, error) {
	st, err := s.Queue.GetSettings(ctx, s.Store.PG, orgID)
	if err != nil {
		return st, NotFound("no settings")
	}
	return st, nil
}

func (s *NotifyService) PatchSettings(ctx context.Context, orgID int64, raw map[string]interface{}) {
	s.Queue.PatchSettings(ctx, s.Store.PG, orgID, raw)
}
