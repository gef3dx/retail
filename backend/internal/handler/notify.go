package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/notify"
	"retail-backend/internal/store"
)

type Notify struct {
	Store *store.Store
}

// Inbox — входящие WEB текущего пользователя.
func (h *Notify) Inbox(c echo.Context) error {
	x := middleware.CtxOf(c)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, notification_type_code, subject, body, entity_type, entity_id, status, sent_at::text
		FROM notification_history
		WHERE recipient_user_id=$1 AND channel_code='WEB'
		ORDER BY id DESC LIMIT 50`, x.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var ntype, ts, st string
		var subj, body, entity *string
		var eid *int64
		_ = rows.Scan(&id, &ntype, &subj, &body, &entity, &eid, &st, &ts)
		out = append(out, map[string]interface{}{"id": id, "type": ntype, "subject": subj,
			"body": body, "entity": entity, "entity_id": eid, "status": st, "at": ts})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Notify) MarkViewed(c echo.Context) error {
	x := middleware.CtxOf(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	res, err := h.Store.PG.Exec(c.Request().Context(), `
		UPDATE notification_history SET status='VIEWED', viewed_at=NOW()
		WHERE id=$1 AND recipient_user_id=$2 AND channel_code='WEB'`, id, x.UserID)
	if err != nil || res.RowsAffected() == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "viewed"})
}

func (h *Notify) Queue(c echo.Context) error {
	status := c.QueryParam("status")
	q := `SELECT id, notification_type_code, channel_code, recipient_name, status,
		attempt_count, scheduled_at::text, priority, entity_type, entity_id
		FROM notification_queue`
	var args []interface{}
	if status != "" {
		q += ` WHERE status=$1`
		args = append(args, status)
	}
	q += ` ORDER BY priority DESC, id DESC LIMIT 100`
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var nt, ch, st, ts string
		var name, entity *string
		var eid *int64
		var att, pri int
		_ = rows.Scan(&id, &nt, &ch, &name, &st, &att, &ts, &pri, &entity, &eid)
		out = append(out, map[string]interface{}{"id": id, "type": nt, "channel": ch,
			"to": name, "status": st, "attempts": att, "scheduled": ts,
			"priority": pri, "entity": entity, "entity_id": eid})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Notify) Templates(c echo.Context) error {
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, notification_type_code, channel_code, name, subject,
		       LEFT(body_template, 120), is_active
		FROM notification_template ORDER BY notification_type_code, channel_code`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var nt, ch, name string
		var subj, preview *string
		var active bool
		_ = rows.Scan(&id, &nt, &ch, &name, &subj, &preview, &active)
		out = append(out, map[string]interface{}{"id": id, "type": nt, "channel": ch,
			"name": name, "subject": subj, "preview": preview, "active": active})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Notify) UpsertTemplate(c echo.Context) error {
	var b struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		Name    string `json:"name"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.Bind(&b); err != nil || b.Type == "" || b.Channel == "" || b.Body == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "type/channel/body required"})
	}
	if b.Name == "" {
		b.Name = b.Type + "/" + b.Channel
	}
	_, err := h.Store.PG.Exec(c.Request().Context(), `
		INSERT INTO notification_template(notification_type_code, channel_code, name, subject, body_template)
		VALUES($1,$2,$3,NULLIF($4,''),$5)
		ON CONFLICT (notification_type_code, channel_code)
		DO UPDATE SET name=$3, subject=NULLIF($4,''), body_template=$5, is_active=TRUE`,
		b.Type, b.Channel, b.Name, b.Subject, b.Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "upsert failed (bad type/channel?)"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Notify) Preferences(c echo.Context) error {
	x := middleware.CtxOf(c)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT notification_type_code, channel_code, enabled FROM notification_preference
		WHERE user_id=$1 ORDER BY 1, 2`, x.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var t, ch string
		var en bool
		_ = rows.Scan(&t, &ch, &en)
		out = append(out, map[string]interface{}{"type": t, "channel": ch, "enabled": en})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Notify) SetPreference(c echo.Context) error {
	x := middleware.CtxOf(c)
	var b struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.Bind(&b); err != nil || b.Type == "" || b.Channel == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "type/channel required"})
	}
	_, err := h.Store.PG.Exec(c.Request().Context(), `
		INSERT INTO notification_preference(user_id, notification_type_code, channel_code, enabled)
		VALUES($1,$2,$3,$4)
		ON CONFLICT (user_id, notification_type_code, channel_code)
		DO UPDATE SET enabled=$4`, x.UserID, b.Type, b.Channel, b.Enabled)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "save failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

type sendReq struct {
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

// Send — ручная постановка (акции, объявления).
func (h *Notify) Send(c echo.Context) error {
	var b sendReq
	if err := c.Bind(&b); err != nil || b.OrgID == 0 || b.Type == "" || len(b.Channels) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id/type/channels required"})
	}
	notify.Enqueue(h.Store, c.Request().Context(), b.OrgID, b.Type, b.Channels,
		notify.Recipient{UserID: b.UserID, Name: b.Name, Email: b.Email, Phone: b.Phone},
		b.Subject, b.Body, b.Data, b.Entity, b.EntityID, 5)
	return c.JSON(http.StatusCreated, map[string]string{"status": "queued"})
}

func (h *Notify) GetSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var enabled bool
	var maxRet, failFirst int
	var threshold float64
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT enabled, max_attempts, fail_first_attempts, low_stock_threshold
		FROM notify_settings WHERE organization_id=$1`, orgID).
		Scan(&enabled, &maxRet, &failFirst, &threshold)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no settings"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"enabled": enabled,
		"max_attempts": maxRet, "fail_first_attempts": failFirst, "low_stock_threshold": threshold})
}

func (h *Notify) PatchSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if v, ok := raw["fail_first_attempts"].(float64); ok {
		_, _ = h.Store.PG.Exec(c.Request().Context(), `
			UPDATE notify_settings SET fail_first_attempts=$1 WHERE organization_id=$2`, int(v), orgID)
	}
	if v, ok := raw["enabled"].(bool); ok {
		_, _ = h.Store.PG.Exec(c.Request().Context(), `
			UPDATE notify_settings SET enabled=$1 WHERE organization_id=$2`, v, orgID)
	}
	if v, ok := raw["low_stock_threshold"].(float64); ok {
		_, _ = h.Store.PG.Exec(c.Request().Context(), `
			UPDATE notify_settings SET low_stock_threshold=$1 WHERE organization_id=$2`, v, orgID)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
