package booking

import (
	"context"
	"log/slog"
	"time"

	"retail-backend/internal/notify"
	"retail-backend/internal/store"
)

// Worker шлет BOOKING_REMINDER по CONFIRMED-броням, стартующим в ближайшие 24ч.
// Идемпотентность — флаг notification_sent.
func Worker(ctx context.Context, s *store.Store, interval time.Duration) {
	if s.PG == nil {
		slog.Error("booking remind worker: no PG, disabled")
		return
	}
	slog.Info("booking remind worker started", "interval", interval.String())
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("booking remind worker stopped")
			return
		case <-t.C:
			remindDue(ctx, s)
		}
	}
}

func remindDue(ctx context.Context, s *store.Store) {
	rows, err := s.PG.Query(ctx, `
		SELECT b.id, b.organization_id, b.created_by_id, p.name, b.start_datetime
		FROM service_booking b
		JOIN service_booking_item bi ON bi.booking_id = b.id
		JOIN catalog_product p ON p.id = bi.product_id
		WHERE b.status_code = 'CONFIRMED' AND NOT b.notification_sent
		  AND b.start_datetime > NOW() AND b.start_datetime <= NOW() + INTERVAL '24 hours'`)
	if err != nil {
		slog.Error("booking remind poll failed", "err", err)
		return
	}
	type due struct {
		id, org int64
		by      *int64
		svc     string
		start   time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.org, &d.by, &d.svc, &d.start); err != nil {
			continue
		}
		list = append(list, d)
	}
	rows.Close()
	for _, d := range list {
		if d.by == nil {
			continue
		}
		notify.EnqueueUser(s, ctx, d.org, "BOOKING_REMINDER", []string{"WEB", "EMAIL"}, *d.by,
			map[string]interface{}{"booking_id": d.id, "service_name": d.svc,
				"start_datetime": d.start.Format("2006-01-02 15:04")}, "booking", &d.id)
		_, _ = s.PG.Exec(ctx, `UPDATE service_booking SET notification_sent=TRUE WHERE id=$1`, d.id)
		slog.Info("booking reminder queued", "booking", d.id)
	}
}
