package repository

import (
	"context"
	"encoding/json"

	"retail-backend/internal/model"
)

// NotifyRepo — очередь уведомлений, предпочтения, low-stock.
// Все методы работают через DBTX (внутри транзакций вызывающего кода).
type NotifyRepo struct{}

// RecipientOf подтягивает контакты пользователя.
func (NotifyRepo) RecipientOf(ctx context.Context, db DBTX, uid int64) model.Recipient {
	r := model.Recipient{UserID: uid}
	_ = db.QueryRow(ctx, `
		SELECT first_name || ' ' || last_name, email, phone FROM users WHERE id=$1`, uid).
		Scan(&r.Name, &r.Email, &r.Phone)
	return r
}

// EnqueueTx ставит уведомление в очередь с учетом предпочтений пользователя.
// Каналы, отключенные в notification_preference, пропускаются.
func (NotifyRepo) EnqueueTx(ctx context.Context, db DBTX, orgID int64, typeCode string,
	channels []string, r model.Recipient, subject, body string,
	data map[string]interface{}, entity string, entityID *int64, priority int) {
	var raw []byte
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	if priority == 0 {
		priority = 5
	}
	for _, ch := range channels {
		if r.UserID != 0 {
			var enabled bool
			_ = db.QueryRow(ctx, `
				SELECT COALESCE(
					(SELECT enabled FROM notification_preference
					 WHERE user_id=$1 AND notification_type_code=$2 AND channel_code=$3), TRUE)`,
				r.UserID, typeCode, ch).Scan(&enabled)
			if !enabled {
				continue
			}
		}
		_, _ = db.Exec(ctx, `
			INSERT INTO notification_queue(organization_id, notification_type_code, channel_code,
				recipient_name, recipient_email, recipient_phone, recipient_user_id,
				subject, body, template_data, entity_type, entity_id, priority)
			VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,0),
				NULLIF($8,''),NULLIF($9,''),$10,NULLIF($11,''),$12,$13)`,
			orgID, typeCode, ch, r.Name, r.Email, r.Phone,
			nullableID(r.UserID), subject, body, raw, entity, entityID, priority)
	}
}

func nullableID(uid int64) interface{} {
	if uid == 0 {
		return nil
	}
	return uid
}

// Admins возвращает id пользователей с ролью ADMIN/SUPER_ADMIN в организации.
func (NotifyRepo) Admins(ctx context.Context, db DBTX, orgID int64) []int64 {
	rows, err := db.Query(ctx, `
		SELECT DISTINCT ur.user_id FROM user_roles ur JOIN roles r ON r.id=ur.role_id
		WHERE r.name IN ('ADMIN','SUPER_ADMIN')
		  AND (ur.organization_id=$1 OR ur.organization_id IS NULL)`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		out = append(out, id)
	}
	return out
}

// EnsureSettings — дефолтные настройки уведомлений организации.
func (NotifyRepo) EnsureSettings(ctx context.Context, db DBTX, orgID int64) {
	_, _ = db.Exec(ctx, `INSERT INTO notify_settings(organization_id) VALUES($1) ON CONFLICT DO NOTHING`, orgID)
}

// CheckLowStock ставит STOCK_LOW админам по товарам ниже порога.
// Дедупликация: не чаще одного уведомления на товар за 24ч.
func (n NotifyRepo) CheckLowStock(ctx context.Context, db DBTX, orgID, warehouseID int64) {
	var thr float64 = 10
	_ = db.QueryRow(ctx, `SELECT low_stock_threshold FROM notify_settings WHERE organization_id=$1`, orgID).Scan(&thr)
	var whName string
	_ = db.QueryRow(ctx, `SELECT name FROM warehouse WHERE id=$1`, warehouseID).Scan(&whName)
	rows, err := db.Query(ctx, `
		SELECT b.product_id, p.name, p.sku, (b.quantity - b.reserved_quantity)
		FROM warehouse_balance b JOIN catalog_product p ON p.id=b.product_id
		WHERE b.warehouse_id=$1 AND (b.quantity - b.reserved_quantity) < $2`, warehouseID, thr)
	if err != nil {
		return
	}
	type low struct {
		pid       int64
		name, sku string
		avail     float64
	}
	var lows []low
	for rows.Next() {
		var l low
		_ = rows.Scan(&l.pid, &l.name, &l.sku, &l.avail)
		lows = append(lows, l)
	}
	rows.Close()
	if len(lows) == 0 {
		return
	}
	admins := n.Admins(ctx, db, orgID)
	for _, l := range lows {
		var recent bool
		_ = db.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM notification_queue
				WHERE notification_type_code='STOCK_LOW' AND entity_type='stock_low'
				  AND entity_id=$1 AND created_at > NOW() - INTERVAL '24 hours')
			OR EXISTS(SELECT 1 FROM notification_history
				WHERE notification_type_code='STOCK_LOW' AND entity_type='stock_low'
				  AND entity_id=$1 AND sent_at > NOW() - INTERVAL '24 hours')`, l.pid).Scan(&recent)
		if recent {
			continue
		}
		data := map[string]interface{}{
			"product_name": l.name, "sku": l.sku,
			"available": l.avail, "warehouse": whName,
		}
		for _, uid := range admins {
			r := n.RecipientOf(ctx, db, uid)
			n.EnqueueTx(ctx, db, orgID, "STOCK_LOW", []string{"WEB", "EMAIL"},
				r, "", "", data, "stock_low", &l.pid, 8)
		}
	}
}

// Inbox возвращает входящие WEB пользователя.
func (NotifyRepo) Inbox(ctx context.Context, db DBTX, userID int64) []model.InboxItem {
	rows, err := db.Query(ctx, `
		SELECT id, notification_type_code, subject, body, entity_type, entity_id, status, sent_at::text
		FROM notification_history
		WHERE recipient_user_id=$1 AND channel_code='WEB'
		ORDER BY id DESC LIMIT 50`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.InboxItem
	for rows.Next() {
		var it model.InboxItem
		_ = rows.Scan(&it.ID, &it.Type, &it.Subject, &it.Body, &it.Entity, &it.EntityID, &it.Status, &it.At)
		out = append(out, it)
	}
	if out == nil {
		out = []model.InboxItem{}
	}
	return out
}

func (NotifyRepo) MarkViewed(ctx context.Context, db DBTX, id, userID int64) bool {
	res, err := db.Exec(ctx, `
		UPDATE notification_history SET status='VIEWED', viewed_at=NOW()
		WHERE id=$1 AND recipient_user_id=$2 AND channel_code='WEB'`, id, userID)
	if err != nil || res.RowsAffected() == 0 {
		return false
	}
	return true
}

func (NotifyRepo) QueueList(ctx context.Context, db DBTX, status string) []model.QueuedItem {
	q := `SELECT id, notification_type_code, channel_code, recipient_name, status,
		attempt_count, scheduled_at::text, priority, entity_type, entity_id
		FROM notification_queue`
	var args []interface{}
	if status != "" {
		q += ` WHERE status=$1`
		args = append(args, status)
	}
	q += ` ORDER BY priority DESC, id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.QueuedItem
	for rows.Next() {
		var it model.QueuedItem
		_ = rows.Scan(&it.ID, &it.Type, &it.Channel, &it.To, &it.Status, &it.Attempts, &it.Scheduled, &it.Priority, &it.Entity, &it.EntityID)
		out = append(out, it)
	}
	if out == nil {
		out = []model.QueuedItem{}
	}
	return out
}

func (NotifyRepo) Templates(ctx context.Context, db DBTX) []model.Template {
	rows, err := db.Query(ctx, `
		SELECT id, notification_type_code, channel_code, name, subject,
		       LEFT(body_template, 120), is_active
		FROM notification_template ORDER BY notification_type_code, channel_code`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Template
	for rows.Next() {
		var t model.Template
		_ = rows.Scan(&t.ID, &t.Type, &t.Channel, &t.Name, &t.Subject, &t.Preview, &t.Active)
		out = append(out, t)
	}
	if out == nil {
		out = []model.Template{}
	}
	return out
}

func (NotifyRepo) UpsertTemplate(ctx context.Context, db DBTX, typ, channel, name, subject, body string) error {
	if name == "" {
		name = typ + "/" + channel
	}
	_, err := db.Exec(ctx, `
		INSERT INTO notification_template(notification_type_code, channel_code, name, subject, body_template)
		VALUES($1,$2,$3,NULLIF($4,''),$5)
		ON CONFLICT (notification_type_code, channel_code)
		DO UPDATE SET name=$3, subject=NULLIF($4,''), body_template=$5, is_active=TRUE`,
		typ, channel, name, subject, body)
	return err
}

func (NotifyRepo) Preferences(ctx context.Context, db DBTX, userID int64) []model.Preference {
	rows, err := db.Query(ctx, `
		SELECT notification_type_code, channel_code, enabled FROM notification_preference
		WHERE user_id=$1 ORDER BY 1, 2`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Preference
	for rows.Next() {
		var p model.Preference
		_ = rows.Scan(&p.Type, &p.Channel, &p.Enabled)
		out = append(out, p)
	}
	if out == nil {
		out = []model.Preference{}
	}
	return out
}

func (NotifyRepo) SetPreference(ctx context.Context, db DBTX, userID int64, typ, channel string, enabled bool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO notification_preference(user_id, notification_type_code, channel_code, enabled)
		VALUES($1,$2,$3,$4)
		ON CONFLICT (user_id, notification_type_code, channel_code)
		DO UPDATE SET enabled=$4`, userID, typ, channel, enabled)
	return err
}

func (NotifyRepo) GetSettings(ctx context.Context, db DBTX, orgID int64) (model.NotifySettings, error) {
	var s model.NotifySettings
	err := db.QueryRow(ctx, `
		SELECT enabled, max_attempts, fail_first_attempts, low_stock_threshold
		FROM notify_settings WHERE organization_id=$1`, orgID).
		Scan(&s.Enabled, &s.MaxAttempts, &s.FailFirstAttempts, &s.LowStockThreshold)
	return s, err
}

func (NotifyRepo) PatchSettings(ctx context.Context, db DBTX, orgID int64, raw map[string]interface{}) {
	if v, ok := raw["fail_first_attempts"].(float64); ok {
		_, _ = db.Exec(ctx, `UPDATE notify_settings SET fail_first_attempts=$1 WHERE organization_id=$2`, int(v), orgID)
	}
	if v, ok := raw["enabled"].(bool); ok {
		_, _ = db.Exec(ctx, `UPDATE notify_settings SET enabled=$1 WHERE organization_id=$2`, v, orgID)
	}
	if v, ok := raw["low_stock_threshold"].(float64); ok {
		_, _ = db.Exec(ctx, `UPDATE notify_settings SET low_stock_threshold=$1 WHERE organization_id=$2`, v, orgID)
	}
}

// --- Worker support ---

// DueJob — задание воркера уведомлений.
type DueJob struct {
	ID                         int64
	OrgID                      int64
	Type, Channel              string
	Name, Email, Phone         *string
	UserID                     *int64
	Subject, Body              *string
	TemplateData               []byte
	Entity                     *string
	EntityID                   *int64
	Attempt, MaxRet, FailFirst int
	Enabled                    bool
}

func (NotifyRepo) PollDue(ctx context.Context, db DBTX) []DueJob {
	rows, err := db.Query(ctx, `
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
		return nil
	}
	defer rows.Close()
	var out []DueJob
	for rows.Next() {
		var j DueJob
		if err := rows.Scan(&j.ID, &j.OrgID, &j.Type, &j.Channel, &j.Name, &j.Email, &j.Phone,
			&j.UserID, &j.Subject, &j.Body, &j.TemplateData, &j.Entity, &j.EntityID,
			&j.Attempt, &j.MaxRet, &j.FailFirst, &j.Enabled); err != nil {
			continue
		}
		out = append(out, j)
	}
	return out
}

func (NotifyRepo) FailMark(ctx context.Context, db DBTX, id int64, attempt, maxRet int) {
	_, _ = db.Exec(ctx, `
		UPDATE notification_queue SET attempt_count=$1, last_attempt_at=NOW(),
			status=CASE WHEN $1::int >= $2::int THEN 'FAILED' ELSE 'RETRY' END,
			error_message='mock: provider unavailable' WHERE id=$3`,
		attempt, maxRet, id)
}

// DrainToHistory переносит отправленное в историю и удаляет из очереди.
func (NotifyRepo) DrainToHistory(ctx context.Context, db DBTX, j DueJob, subject, body, status, provID string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO notification_history(organization_id, notification_type_code, channel_code,
			recipient_name, recipient_email, recipient_phone, recipient_user_id,
			subject, body, entity_type, entity_id, status, provider_message_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		j.OrgID, j.Type, j.Channel, j.Name, j.Email, j.Phone, j.UserID,
		subject, body, j.Entity, j.EntityID, status, provID)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM notification_queue WHERE id=$1`, j.ID)
	return err
}

// TemplateFor возвращает subject/body шаблона.
func (NotifyRepo) TemplateFor(ctx context.Context, db DBTX, ntype, channel string) (subj *string, body string) {
	_ = db.QueryRow(ctx, `
		SELECT subject, body_template FROM notification_template
		WHERE notification_type_code=$1 AND channel_code=$2 AND is_active LIMIT 1`,
		ntype, channel).Scan(&subj, &body)
	return subj, body
}
