package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/store"
)

var varRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// Render подставляет {{var}} из data.
func Render(tpl string, data map[string]interface{}) string {
	return varRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := varRe.FindStringSubmatch(m)[1]
		if v, ok := data[key]; ok && v != nil {
			return sprint(v)
		}
		return m
	})
}

func sprint(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return string(t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

type Recipient struct {
	UserID int64
	Name   string
	Email  string
	Phone  string
}

// userRecipient подтягивает контакты пользователя.
func userRecipient(ctx context.Context, s *store.Store, uid int64) Recipient {
	r := Recipient{UserID: uid}
	_ = s.PG.QueryRow(ctx, `
		SELECT first_name || ' ' || last_name, email, phone FROM users WHERE id=$1`, uid).
		Scan(&r.Name, &r.Email, &r.Phone)
	return r
}

// RecipientOf — экспортируемый резолв получателя по user_id.
func RecipientOf(ctx context.Context, s *store.Store, uid int64) Recipient {
	return userRecipient(ctx, s, uid)
}

// EnqueueTx ставит уведомление в очередь с учетом предпочтений пользователя.
// Каналы, отключенные в notification_preference, пропускаются.
func EnqueueTx(tx pgx.Tx, ctx context.Context, orgID int64, typeCode string,
	channels []string, r Recipient, subject, body string,
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
			_ = tx.QueryRow(ctx, `
				SELECT COALESCE(
					(SELECT enabled FROM notification_preference
					 WHERE user_id=$1 AND notification_type_code=$2 AND channel_code=$3), TRUE)`,
				r.UserID, typeCode, ch).Scan(&enabled)
			if !enabled {
				continue
			}
		}
		_, _ = tx.Exec(ctx, `
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

// Enqueue — вариант вне транзакции.
func Enqueue(s *store.Store, ctx context.Context, orgID int64, typeCode string,
	channels []string, r Recipient, subject, body string,
	data map[string]interface{}, entity string, entityID *int64, priority int) {
	tx, err := s.PG.Begin(ctx)
	if err != nil {
		slog.Error("notify enqueue begin failed", "err", err)
		return
	}
	defer tx.Rollback(ctx)
	EnqueueTx(tx, ctx, orgID, typeCode, channels, r, subject, body, data, entity, entityID, priority)
	_ = tx.Commit(ctx)
}

// EnqueueUser — удобный вариант для уведомления пользователя (контакты из users).
func EnqueueUser(s *store.Store, ctx context.Context, orgID int64, typeCode string,
	channels []string, uid int64, data map[string]interface{}, entity string, entityID *int64) {
	Enqueue(s, ctx, orgID, typeCode, channels, userRecipient(ctx, s, uid), "", "", data, entity, entityID, 5)
}

// Admins возвращает id пользователей с ролью ADMIN/SUPER_ADMIN в организации.
func Admins(ctx context.Context, s *store.Store, orgID int64) []int64 {
	rows, err := s.PG.Query(ctx, `
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
func EnsureSettings(ctx context.Context, s *store.Store, orgID int64) {
	_, _ = s.PG.Exec(ctx, `INSERT INTO notify_settings(organization_id) VALUES($1) ON CONFLICT DO NOTHING`, orgID)
}

// CheckLowStockTx ставит STOCK_LOW админам по товарам ниже порога.
// Дедупликация: не чаще одного уведомления на товар за 24ч. Вызывать в транзакции
// после операций, уменьшающих остатки.
func CheckLowStockTx(tx pgx.Tx, ctx context.Context, s *store.Store, orgID, warehouseID int64) {
	var thr float64 = 10
	_ = tx.QueryRow(ctx, `SELECT low_stock_threshold FROM notify_settings WHERE organization_id=$1`, orgID).Scan(&thr)
	var whName string
	_ = tx.QueryRow(ctx, `SELECT name FROM warehouse WHERE id=$1`, warehouseID).Scan(&whName)
	rows, err := tx.Query(ctx, `
		SELECT b.product_id, p.name, p.sku, (b.quantity - b.reserved_quantity)
		FROM warehouse_balance b JOIN catalog_product p ON p.id=b.product_id
		WHERE b.warehouse_id=$1 AND (b.quantity - b.reserved_quantity) < $2`, warehouseID, thr)
	if err != nil {
		return
	}
	type low struct {
		pid         int64
		name, sku   string
		avail       float64
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
	admins := Admins(ctx, s, orgID)
	for _, l := range lows {
		var recent bool
		_ = tx.QueryRow(ctx, `
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
			r := userRecipient(ctx, s, uid)
			EnqueueTx(tx, ctx, orgID, "STOCK_LOW", []string{"WEB", "EMAIL"},
				r, "", "", data, "stock_low", &l.pid, 8)
		}
	}
}
