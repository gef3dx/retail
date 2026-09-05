package repository

import (
	"context"
	"time"

	"retail-backend/internal/model"
)

// ResourceRepo — ресурсы и расписание.
type ResourceRepo struct{}

func (ResourceRepo) List(ctx context.Context, db DBTX, orgID int64) []model.Resource {
	rows, err := db.Query(ctx, `
		SELECT id, resource_type_code, name, user_id, location, is_active
		FROM service_resource WHERE organization_id=$1 ORDER BY id`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Resource
	for rows.Next() {
		var r model.Resource
		_ = rows.Scan(&r.ID, &r.Type, &r.Name, &r.UserID, &r.Location, &r.IsActive)
		out = append(out, r)
	}
	if out == nil {
		out = []model.Resource{}
	}
	return out
}

func (ResourceRepo) Create(ctx context.Context, db DBTX, orgID int64, typ, name string, userID *int64, location string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO service_resource(organization_id, resource_type_code, name, user_id, location)
		VALUES($1,$2,$3,$4,NULLIF($5,'')) RETURNING id`,
		orgID, typ, name, userID, location).Scan(&id)
	return id, err
}

func (ResourceRepo) ActiveInOrg(ctx context.Context, db DBTX, resourceID, orgID int64) bool {
	var cnt int
	_ = db.QueryRow(ctx, `
		SELECT COUNT(*) FROM service_resource WHERE id=$1 AND organization_id=$2 AND is_active`, resourceID, orgID).Scan(&cnt)
	return cnt > 0
}

func (ResourceRepo) Schedule(ctx context.Context, db DBTX, resourceID int64) []model.SchedDay {
	rows, err := db.Query(ctx, `
		SELECT day_of_week, start_time::text, end_time::text, is_active
		FROM service_resource_schedule WHERE resource_id=$1 ORDER BY day_of_week`, resourceID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.SchedDay
	for rows.Next() {
		var d model.SchedDay
		var st, en string
		_ = rows.Scan(&d.DOW, &st, &en, &d.Active)
		d.Start = truncHM(st)
		d.End = truncHM(en)
		out = append(out, d)
	}
	if out == nil {
		out = []model.SchedDay{}
	}
	return out
}

func truncHM(s string) string {
	if len(s) > 5 {
		return s[:5]
	}
	return s
}

func (ResourceRepo) SaveScheduleDay(ctx context.Context, db DBTX, resourceID int64, d model.SchedDay) error {
	_, err := db.Exec(ctx, `
		INSERT INTO service_resource_schedule(resource_id, day_of_week, start_time, end_time, is_active)
		VALUES($1,$2,$3::time,$4::time,$5)
		ON CONFLICT (resource_id, day_of_week)
		DO UPDATE SET start_time=$3::time, end_time=$4::time, is_active=$5`,
		resourceID, d.DOW, d.Start, d.End, d.Active)
	return err
}

func (ResourceRepo) AddException(ctx context.Context, db DBTX, resourceID int64, date string, working bool, start, end, reason string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO service_resource_schedule_exception(resource_id, exception_date, is_working, start_time, end_time, reason)
		VALUES($1,$2::date,$3,NULLIF($4,'')::time,NULLIF($5,'')::time,NULLIF($6,''))
		ON CONFLICT (resource_id, exception_date)
		DO UPDATE SET is_working=$3, start_time=NULLIF($4,'')::time, end_time=NULLIF($5,'')::time, reason=NULLIF($6,'')`,
		resourceID, date, working, start, end, reason)
	return err
}

// HasSchedule проверяет, есть ли хоть одна строка расписания у ресурса.
func (ResourceRepo) HasSchedule(ctx context.Context, db DBTX, resourceID int64) bool {
	var has bool
	_ = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM service_resource_schedule WHERE resource_id=$1)`, resourceID).Scan(&has)
	return has
}

// Exception возвращает исключение на дату (found=false если нет).
func (ResourceRepo) Exception(ctx context.Context, db DBTX, resourceID int64, date string) (working bool, start, end string, found bool) {
	var isW bool
	var st, en *string
	if err := db.QueryRow(ctx, `
		SELECT is_working, start_time::text, end_time::text
		FROM service_resource_schedule_exception
		WHERE resource_id=$1 AND exception_date=$2::date`, resourceID, date).Scan(&isW, &st, &en); err != nil {
		return false, "", "", false
	}
	if st != nil {
		start = truncHM(*st)
	}
	if en != nil {
		end = truncHM(*en)
	}
	return isW, start, end, true
}

// DaySchedule возвращает (start, end, active, found) для дня недели.
func (ResourceRepo) DaySchedule(ctx context.Context, db DBTX, resourceID int64, dow int) (string, string, bool, bool) {
	var st, en string
	var active bool
	if err := db.QueryRow(ctx, `
		SELECT start_time::text, end_time::text, is_active
		FROM service_resource_schedule WHERE resource_id=$1 AND day_of_week=$2`, resourceID, dow).
		Scan(&st, &en, &active); err != nil {
		return "", "", false, false
	}
	return truncHM(st), truncHM(en), active, true
}

// BusyIntervals возвращает занятые интервалы ресурса на дату.
func (ResourceRepo) BusyIntervals(ctx context.Context, db DBTX, resourceID int64, date string) [][2]time.Time {
	rows, err := db.Query(ctx, `
		SELECT b.start_datetime, b.end_datetime FROM service_booking b
		JOIN service_booking_resource br ON br.booking_id=b.id
		WHERE br.resource_id=$1 AND b.start_datetime::date=$2::date
		  AND b.status_code NOT IN ('COMPLETED','CANCELED','NO_SHOW')`, resourceID, date)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out [][2]time.Time
	for rows.Next() {
		var s, e time.Time
		_ = rows.Scan(&s, &e)
		out = append(out, [2]time.Time{s, e})
	}
	return out
}

// Overlaps проверяет пересечения с нефинальными бронями (исключая excludeID).
func (ResourceRepo) Overlaps(ctx context.Context, db DBTX, resourceID int64, start, end time.Time, excludeID int64) (bool, error) {
	var n int
	err := db.QueryRow(ctx, `
		SELECT COUNT(*) FROM service_booking b
		JOIN service_booking_resource br ON br.booking_id=b.id
		WHERE br.resource_id=$1 AND b.id <> $4
		  AND b.status_code NOT IN ('COMPLETED','CANCELED','NO_SHOW')
		  AND b.start_datetime < $3 AND b.end_datetime > $2`,
		resourceID, start, end, excludeID).Scan(&n)
	return n > 0, err
}

// MandatoryResources возвращает обязательные ресурсы услуги.
func (ResourceRepo) MandatoryResources(ctx context.Context, db DBTX, productID int64) []int64 {
	rows, err := db.Query(ctx, `
		SELECT resource_id FROM service_product_resource WHERE product_id=$1 AND is_mandatory`, productID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var rid int64
		_ = rows.Scan(&rid)
		out = append(out, rid)
	}
	return out
}

// BookingRepo — бронирования.
type BookingRepo struct{}

// ServiceForBooking возвращает тип/имя/длительность/флаги услуги и цену.
func (BookingRepo) ServiceForBooking(ctx context.Context, db DBTX, productID int64) (ptype, name string, dur *int, requires, enabled bool, basePrice *float64, err error) {
	err = db.QueryRow(ctx, `
		SELECT product_type, name, service_duration_minutes, service_requires_booking,
		       service_booking_enabled, base_price
		FROM catalog_product WHERE id=$1 AND is_active`, productID).
		Scan(&ptype, &name, &dur, &requires, &enabled, &basePrice)
	return ptype, name, dur, requires, enabled, basePrice, err
}

func (BookingRepo) Create(ctx context.Context, db DBTX, orgID int64, customerID *int64,
	custName, custPhone, custEmail string, start, end time.Time, duration int, notes string, createdBy int64) (int64, error) {
	var bid int64
	err := db.QueryRow(ctx, `
		INSERT INTO service_booking(organization_id, customer_id, customer_name, customer_phone,
			customer_email, start_datetime, end_datetime, duration_minutes, notes, created_by_id)
		VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,$8,NULLIF($9,''),$10) RETURNING id`,
		orgID, customerID, custName, custPhone, custEmail, start, end, duration, notes, createdBy).Scan(&bid)
	return bid, err
}

func (BookingRepo) AddItem(ctx context.Context, db DBTX, bookingID, productID int64, price float64, duration int) (int64, error) {
	var itemID int64
	err := db.QueryRow(ctx, `
		INSERT INTO service_booking_item(booking_id, product_id, price, duration_minutes)
		VALUES($1,$2,$3,$4) RETURNING id`, bookingID, productID, price, duration).Scan(&itemID)
	return itemID, err
}

func (BookingRepo) AttachResource(ctx context.Context, db DBTX, bookingID, resourceID, itemID int64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO service_booking_resource(booking_id, resource_id, booking_item_id)
		VALUES($1,$2,$3)`, bookingID, resourceID, itemID)
	return err
}

func (BookingRepo) History(ctx context.Context, db DBTX, bookingID int64, status string, by int64, comment string) {
	_, _ = db.Exec(ctx, `
		INSERT INTO service_booking_status_history(booking_id, status_code, changed_by_id, comment)
		VALUES($1,$2,$3,NULLIF($4,''))`, bookingID, status, by, comment)
}

// BookingFilter — фильтры журнала.
type BookingFilter struct {
	OrgID  int64
	Date   string
	Status string
}

func (BookingRepo) List(ctx context.Context, db DBTX, f BookingFilter) []model.Booking {
	q := `SELECT b.id, COALESCE(b.customer_name,''), b.start_datetime::text, b.end_datetime::text,
		b.status_code, COALESCE(p.name,''), COALESCE(string_agg(r.name, ', '),'')
		FROM service_booking b
		LEFT JOIN service_booking_item bi ON bi.booking_id=b.id
		LEFT JOIN catalog_product p ON p.id=bi.product_id
		LEFT JOIN service_booking_resource br ON br.booking_id=b.id
		LEFT JOIN service_resource r ON r.id=br.resource_id
		WHERE b.organization_id=$1`
	args := []interface{}{f.OrgID}
	if f.Date != "" {
		args = append(args, f.Date)
		q += ` AND b.start_datetime::date = $` + itoa(len(args)) + `::date`
	}
	if f.Status != "" {
		args = append(args, f.Status)
		q += ` AND b.status_code=$` + itoa(len(args))
	}
	q += ` GROUP BY b.id, p.name ORDER BY b.start_datetime LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Booking
	for rows.Next() {
		var b model.Booking
		_ = rows.Scan(&b.ID, &b.Customer, &b.Start, &b.End, &b.Status, &b.Service, &b.Resources)
		out = append(out, b)
	}
	if out == nil {
		out = []model.Booking{}
	}
	return out
}

func (BookingRepo) Get(ctx context.Context, db DBTX, id int64) (model.BookingDetail, error) {
	var d model.BookingDetail
	if err := db.QueryRow(ctx, `
		SELECT organization_id, COALESCE(customer_name,''), COALESCE(customer_phone,''),
		       COALESCE(customer_email,''), COALESCE(notes,''), status_code,
		       start_datetime::text, end_datetime::text, duration_minutes
		FROM service_booking WHERE id=$1`, id).
		Scan(&d.OrgID, &d.Customer, &d.Phone, &d.Email, &d.Notes, &d.Status, &d.Start, &d.End, &d.Duration); err != nil {
		return d, err
	}
	d.ID = id
	rows, err := db.Query(ctx, `
		SELECT status_code, changed_by_id, comment, changed_at::text
		FROM service_booking_status_history WHERE booking_id=$1 ORDER BY id`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var h model.BookingHistory
		_ = rows.Scan(&h.Status, &h.By, &h.Comment, &h.At)
		d.History = append(d.History, h)
	}
	if d.History == nil {
		d.History = []model.BookingHistory{}
	}
	return d, nil
}

func (BookingRepo) StatusOf(ctx context.Context, db DBTX, id int64) (cur string, orgID int64, err error) {
	err = db.QueryRow(ctx, `SELECT status_code, organization_id FROM service_booking WHERE id=$1 FOR UPDATE`, id).Scan(&cur, &orgID)
	return cur, orgID, err
}

func (BookingRepo) SetStatus(ctx context.Context, db DBTX, id int64, status string) {
	_, _ = db.Exec(ctx, `
		UPDATE service_booking SET status_code=$2,
			confirmed_at = CASE WHEN $2::varchar='CONFIRMED' THEN NOW() ELSE confirmed_at END,
			completed_at = CASE WHEN $2::varchar='COMPLETED' THEN NOW() ELSE completed_at END,
			canceled_at = CASE WHEN $2::varchar IN ('CANCELED','NO_SHOW') THEN NOW() ELSE canceled_at END
		WHERE id=$1`, id, status)
}

// ConfirmInfo для уведомления о подтверждении.
func (BookingRepo) ConfirmInfo(ctx context.Context, db DBTX, id int64) (svc, start string) {
	_ = db.QueryRow(ctx, `
		SELECT p.name, b.start_datetime::text FROM service_booking b
		JOIN service_booking_item bi ON bi.booking_id=b.id
		JOIN catalog_product p ON p.id=bi.product_id WHERE b.id=$1 LIMIT 1`, id).Scan(&svc, &start)
	return svc, start
}

func (BookingRepo) LinkReceipt(ctx context.Context, db DBTX, bookingID, itemID int64) error {
	res, err := db.Exec(ctx, `
		UPDATE service_booking SET sales_receipt_item_id=$2 WHERE id=$1
		AND EXISTS(SELECT 1 FROM sales_receipt_item WHERE id=$2)`, bookingID, itemID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errNotFound
	}
	return nil
}

func (BookingRepo) LinkProductResource(ctx context.Context, db DBTX, productID, resourceID int64, mandatory bool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO service_product_resource(product_id, resource_id, is_mandatory)
		VALUES($1,$2,$3) ON CONFLICT (product_id, resource_id)
		DO UPDATE SET is_mandatory=$3`, productID, resourceID, mandatory)
	return err
}

func (BookingRepo) ProductResources(ctx context.Context, db DBTX, productID int64) []model.ProductResource {
	rows, err := db.Query(ctx, `
		SELECT r.id, r.name, r.resource_type_code, pr.is_mandatory
		FROM service_product_resource pr JOIN service_resource r ON r.id=pr.resource_id
		WHERE pr.product_id=$1`, productID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.ProductResource
	for rows.Next() {
		var r model.ProductResource
		_ = rows.Scan(&r.ID, &r.Name, &r.Type, &r.Mandatory)
		out = append(out, r)
	}
	if out == nil {
		out = []model.ProductResource{}
	}
	return out
}

// DueReminders — CONFIRMED-брони на ближайшие 24ч без отправленного напоминания.
type DueReminder struct {
	ID    int64
	OrgID int64
	By    *int64
	Svc   string
	Start time.Time
}

func (BookingRepo) DueReminders(ctx context.Context, db DBTX) []DueReminder {
	rows, err := db.Query(ctx, `
		SELECT b.id, b.organization_id, b.created_by_id, p.name, b.start_datetime
		FROM service_booking b
		JOIN service_booking_item bi ON bi.booking_id = b.id
		JOIN catalog_product p ON p.id = bi.product_id
		WHERE b.status_code = 'CONFIRMED' AND NOT b.notification_sent
		  AND b.start_datetime > NOW() AND b.start_datetime <= NOW() + INTERVAL '24 hours'`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []DueReminder
	for rows.Next() {
		var d DueReminder
		if err := rows.Scan(&d.ID, &d.OrgID, &d.By, &d.Svc, &d.Start); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (BookingRepo) MarkReminded(ctx context.Context, db DBTX, id int64) {
	_, _ = db.Exec(ctx, `UPDATE service_booking SET notification_sent=TRUE WHERE id=$1`, id)
}
