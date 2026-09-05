package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/notify"
	"retail-backend/internal/store"
)

type Booking struct {
	Store *store.Store
}

// ---------- Resources ----------

func (h *Booking) ListResources(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, resource_type_code, name, user_id, location, is_active
		FROM service_resource WHERE organization_id=$1 ORDER BY id`, orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var typ, name string
		var uid *int64
		var loc *string
		var active bool
		_ = rows.Scan(&id, &typ, &name, &uid, &loc, &active)
		out = append(out, map[string]interface{}{"id": id, "type": typ, "name": name,
			"user_id": uid, "location": loc, "is_active": active})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Booking) CreateResource(c echo.Context) error {
	var b struct {
		OrgID    int64  `json:"org_id"`
		Type     string `json:"type"`
		Name     string `json:"name"`
		UserID   *int64 `json:"user_id"`
		Location string `json:"location"`
	}
	if err := c.Bind(&b); err != nil || b.OrgID == 0 || b.Type == "" || b.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id/type/name required"})
	}
	var id int64
	if err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO service_resource(organization_id, resource_type_code, name, user_id, location)
		VALUES($1,$2,$3,$4,NULLIF($5,'')) RETURNING id`,
		b.OrgID, b.Type, b.Name, b.UserID, b.Location).Scan(&id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "create failed (bad type?)"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

// ---------- Schedule ----------

type schedDay struct {
	DOW    int    `json:"dow"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Active bool   `json:"active"`
}

func (h *Booking) GetSchedule(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT day_of_week, start_time::text, end_time::text, is_active
		FROM service_resource_schedule WHERE resource_id=$1 ORDER BY day_of_week`, rid)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var dow int
		var st, en string
		var active bool
		_ = rows.Scan(&dow, &st, &en, &active)
		out = append(out, map[string]interface{}{"dow": dow, "start": st[:5], "end": en[:5], "active": active})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Booking) PutSchedule(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Days []schedDay `json:"days"`
	}
	if err := c.Bind(&b); err != nil || len(b.Days) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "days required"})
	}
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		for _, d := range b.Days {
			if d.DOW < 1 || d.DOW > 7 || d.Start == "" || d.End == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "bad day")
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO service_resource_schedule(resource_id, day_of_week, start_time, end_time, is_active)
				VALUES($1,$2,$3::time,$4::time,$5)
				ON CONFLICT (resource_id, day_of_week)
				DO UPDATE SET start_time=$3::time, end_time=$4::time, is_active=$5`,
				rid, d.DOW, d.Start, d.End, d.Active); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "save failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Booking) AddException(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Date      string `json:"date"`
		IsWorking bool   `json:"is_working"`
		Start     string `json:"start"`
		End       string `json:"end"`
		Reason    string `json:"reason"`
	}
	if err := c.Bind(&b); err != nil || b.Date == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "date required"})
	}
	_, err := h.Store.PG.Exec(c.Request().Context(), `
		INSERT INTO service_resource_schedule_exception(resource_id, exception_date, is_working, start_time, end_time, reason)
		VALUES($1,$2::date,$3,NULLIF($4,'')::time,NULLIF($5,'')::time,NULLIF($6,''))
		ON CONFLICT (resource_id, exception_date)
		DO UPDATE SET is_working=$3, start_time=NULLIF($4,'')::time, end_time=NULLIF($5,'')::time, reason=NULLIF($6,'')`,
		rid, b.Date, b.IsWorking, b.Start, b.End, b.Reason)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "save failed"})
	}
	return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
}

// ---------- Availability ----------

// resourceBusy проверяет пересечения с нефинальными бронями ресурса.
func resourceBusy(tx pgx.Tx, ctx context.Context, resourceID int64, start, end time.Time, excludeBooking int64) (bool, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM service_booking b
		JOIN service_booking_resource br ON br.booking_id=b.id
		WHERE br.resource_id=$1 AND b.id <> $4
		  AND b.status_code NOT IN ('COMPLETED','CANCELED','NO_SHOW')
		  AND b.start_datetime < $3 AND b.end_datetime > $2`,
		resourceID, start, end, excludeBooking).Scan(&n)
	return n > 0, err
}

// resourceWorking проверяет расписание: исключение на дату важнее недельного.
// Если строк расписания нет вообще — считаем 24/7 (мягкий старт).
func resourceWorking(tx pgx.Tx, ctx context.Context, resourceID int64, start, end time.Time) (bool, string) {
	var hasSched bool
	_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM service_resource_schedule WHERE resource_id=$1)`, resourceID).Scan(&hasSched)
	if !hasSched {
		return true, ""
	}
	date := start.Format("2006-01-02")
	var isWorking *bool
	var st, en *string
	err := tx.QueryRow(ctx, `
		SELECT is_working, start_time::text, end_time::text
		FROM service_resource_schedule_exception
		WHERE resource_id=$1 AND exception_date=$2::date`, resourceID, date).Scan(&isWorking, &st, &en)
	if err == nil {
		if !*isWorking {
			return false, "day off (exception)"
		}
		if st != nil && en != nil {
			return within(*st, *en, start, end), "exception hours"
		}
		return true, ""
	}
	// Go: Monday=0..Sunday=6 → наш dow 1..7.
	dow := int(start.Weekday())
	if dow == 0 {
		dow = 7
	}
	var active bool
	var sst, sen string
	if err := tx.QueryRow(ctx, `
		SELECT start_time::text, end_time::text, is_active
		FROM service_resource_schedule WHERE resource_id=$1 AND day_of_week=$2`, resourceID, dow).
		Scan(&sst, &sen, &active); err != nil || !active {
		return false, "no schedule for weekday"
	}
	return within(sst, sen, start, end), ""
}

func within(sst, sen string, start, end time.Time) bool {
	if len(sst) > 5 {
		sst = sst[:5]
	}
	if len(sen) > 5 {
		sen = sen[:5]
	}
	sf := start.Format("15:04")
	ef := end.Format("15:04")
	return sst <= sf && ef <= sen
}

// ---------- Bookings ----------

type bookingReq struct {
	OrgID       int64   `json:"org_id"`
	ProductID   int64   `json:"product_id"`
	ResourceIDs []int64 `json:"resource_ids"`
	Start       string  `json:"start"`
	Duration    *int    `json:"duration_minutes"`
	CustomerID  *int64  `json:"customer_id"`
	CustName    string  `json:"customer_name"`
	CustPhone   string  `json:"customer_phone"`
	CustEmail   string  `json:"customer_email"`
	Notes       string  `json:"notes"`
	Price       *float64 `json:"price"`
}

func (h *Booking) Create(c echo.Context) error {
	var b bookingReq
	if err := c.Bind(&b); err != nil || b.OrgID == 0 || b.ProductID == 0 || b.Start == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id/product_id/start required"})
	}
	start, err := time.Parse(time.RFC3339, b.Start)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "start must be RFC3339"})
	}
	if start.Before(time.Now().Add(-5 * time.Minute)) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "start in the past"})
	}
	x := middleware.CtxOf(c)
	var bid int64
	err = h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var ptype, pname string
		var dur *int
		var requires, enabled bool
		var basePrice *float64
		if err := tx.QueryRow(ctx, `
			SELECT product_type, name, service_duration_minutes, service_requires_booking,
			       service_booking_enabled, base_price
			FROM catalog_product WHERE id=$1 AND is_active`, b.ProductID).
			Scan(&ptype, &pname, &dur, &requires, &enabled, &basePrice); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no product")
		}
		if ptype != "SERVICE" {
			return echo.NewHTTPError(http.StatusBadRequest, "product is not a service")
		}
		if !enabled {
			return echo.NewHTTPError(http.StatusBadRequest, "booking disabled for service")
		}
		duration := 0
		if b.Duration != nil {
			duration = *b.Duration
		} else if dur != nil {
			duration = *dur
		}
		if duration <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "duration required (product has none)")
		}
		end := start.Add(time.Duration(duration) * time.Minute)
		// Обязательные ресурсы услуги должны быть покрыты.
		rows, err := tx.Query(ctx, `
			SELECT resource_id FROM service_product_resource WHERE product_id=$1 AND is_mandatory`, b.ProductID)
		if err != nil {
			return err
		}
		need := map[int64]bool{}
		for rows.Next() {
			var rid int64
			_ = rows.Scan(&rid)
			need[rid] = true
		}
		rows.Close()
		given := map[int64]bool{}
		for _, rid := range b.ResourceIDs {
			var cnt int
			if err := tx.QueryRow(ctx, `
				SELECT COUNT(*) FROM service_resource WHERE id=$1 AND organization_id=$2 AND is_active`, rid, b.OrgID).Scan(&cnt); err != nil || cnt == 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "bad resource")
			}
			given[rid] = true
		}
		for rid := range need {
			if !given[rid] {
				return echo.NewHTTPError(http.StatusBadRequest, "mandatory resource missing")
			}
		}
		// Проверки по каждому ресурсу.
		for rid := range given {
			if ok, reason := resourceWorking(tx, ctx, rid, start, end); !ok {
				return echo.NewHTTPError(http.StatusConflict, "resource off schedule: "+reason)
			}
			busy, err := resourceBusy(tx, ctx, rid, start, end, 0)
			if err != nil {
				return err
			}
			if busy {
				return echo.NewHTTPError(http.StatusConflict, "resource busy")
			}
		}
		price := 0.0
		if b.Price != nil {
			price = *b.Price
		} else if basePrice != nil {
			price = *basePrice
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO service_booking(organization_id, customer_id, customer_name, customer_phone,
				customer_email, start_datetime, end_datetime, duration_minutes, notes, created_by_id)
			VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,$8,NULLIF($9,''),$10) RETURNING id`,
			b.OrgID, b.CustomerID, b.CustName, b.CustPhone, b.CustEmail,
			start, end, duration, b.Notes, x.UserID).Scan(&bid); err != nil {
			return err
		}
		var itemID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO service_booking_item(booking_id, product_id, price, duration_minutes)
			VALUES($1,$2,$3,$4) RETURNING id`, bid, b.ProductID, price, duration).Scan(&itemID); err != nil {
			return err
		}
		for rid := range given {
			if _, err := tx.Exec(ctx, `
				INSERT INTO service_booking_resource(booking_id, resource_id, booking_item_id)
				VALUES($1,$2,$3)`, bid, rid, itemID); err != nil {
				return err
			}
		}
		_, _ = tx.Exec(ctx, `
			INSERT INTO service_booking_status_history(booking_id, status_code, changed_by_id, comment)
			VALUES($1,'PENDING',$2,'Создано')`, bid, x.UserID)
		notify.EnqueueTx(tx, ctx, b.OrgID, "BOOKING_CREATED", []string{"WEB", "EMAIL"},
			notify.RecipientOf(ctx, h.Store, x.UserID), "", "",
			map[string]interface{}{"booking_id": bid, "service_name": pname,
				"start_datetime": start.Format("2006-01-02 15:04")}, "booking", &bid, 5)
		_ = pname
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "create booking failed"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": bid})
}

func (h *Booking) List(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	date := c.QueryParam("date") // YYYY-MM-DD
	status := c.QueryParam("status")
	q := `SELECT b.id, b.customer_name, b.start_datetime::text, b.end_datetime::text,
		b.status_code, COALESCE(p.name,''), COALESCE(string_agg(r.name, ', '),'')
		FROM service_booking b
		LEFT JOIN service_booking_item bi ON bi.booking_id=b.id
		LEFT JOIN catalog_product p ON p.id=bi.product_id
		LEFT JOIN service_booking_resource br ON br.booking_id=b.id
		LEFT JOIN service_resource r ON r.id=br.resource_id
		WHERE b.organization_id=$1`
	args := []interface{}{orgID}
	if date != "" {
		args = append(args, date)
		q += ` AND b.start_datetime::date = $` + strconv.Itoa(len(args)) + `::date`
	}
	if status != "" {
		args = append(args, status)
		q += ` AND b.status_code=$` + strconv.Itoa(len(args))
	}
	q += ` GROUP BY b.id, p.name ORDER BY b.start_datetime LIMIT 100`
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var cname, st, svc, res, ts, te string
		_ = rows.Scan(&id, &cname, &ts, &te, &st, &svc, &res)
		out = append(out, map[string]interface{}{"id": id, "customer": cname,
			"start": ts, "end": te, "status": st, "service": svc, "resources": res})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Booking) Get(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var org int64
	var cname, phone, email, notes string
	var st, ts, te string
	var dur int
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT organization_id, COALESCE(customer_name,''), COALESCE(customer_phone,''),
		       COALESCE(customer_email,''), COALESCE(notes,''), status_code,
		       start_datetime::text, end_datetime::text, duration_minutes
		FROM service_booking WHERE id=$1`, id).
		Scan(&org, &cname, &phone, &email, &notes, &st, &ts, &te, &dur)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no booking"})
	}
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT s.status_code, s.changed_by_id, s.comment, s.changed_at::text
		FROM service_booking_status_history s WHERE s.booking_id=$1 ORDER BY s.id`, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	hist := []map[string]interface{}{}
	for rows.Next() {
		var code, ts string
		var by *int64
		var cm *string
		_ = rows.Scan(&code, &by, &cm, &ts)
		hist = append(hist, map[string]interface{}{"status": code, "by": by, "comment": cm, "at": ts})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": id, "org_id": org,
		"customer": cname, "phone": phone, "email": email, "notes": notes,
		"status": st, "start": ts, "end": te, "duration": dur, "history": hist})
}

var transitions = map[string][]string{
	"PENDING":     {"CONFIRMED", "CANCELED"},
	"CONFIRMED":   {"IN_PROGRESS", "CANCELED", "NO_SHOW"},
	"IN_PROGRESS": {"COMPLETED", "NO_SHOW"},
}

func (h *Booking) SetStatus(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}
	if err := c.Bind(&b); err != nil || b.Status == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "status required"})
	}
	x := middleware.CtxOf(c)
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var cur, org string
		var orgID int64
		if err := tx.QueryRow(ctx, `SELECT status_code, organization_id FROM service_booking WHERE id=$1 FOR UPDATE`, id).
			Scan(&cur, &orgID); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no booking")
		}
		_ = org
		ok := false
		for _, s := range transitions[cur] {
			if s == b.Status {
				ok = true
			}
		}
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "bad transition "+cur+" -> "+b.Status)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE service_booking SET status_code=$2,
				confirmed_at = CASE WHEN $2::varchar='CONFIRMED' THEN NOW() ELSE confirmed_at END,
				completed_at = CASE WHEN $2::varchar='COMPLETED' THEN NOW() ELSE completed_at END,
				canceled_at = CASE WHEN $2::varchar IN ('CANCELED','NO_SHOW') THEN NOW() ELSE canceled_at END
			WHERE id=$1`, id, b.Status); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `
			INSERT INTO service_booking_status_history(booking_id, status_code, changed_by_id, comment)
			VALUES($1,$2,$3,NULLIF($4,''))`, id, b.Status, x.UserID, b.Comment)
		if b.Status == "CONFIRMED" {
			var svc, st string
			var bbid int64
			_ = tx.QueryRow(ctx, `
				SELECT p.name, b.start_datetime::text, b.id FROM service_booking b
				JOIN service_booking_item bi ON bi.booking_id=b.id
				JOIN catalog_product p ON p.id=bi.product_id WHERE b.id=$1 LIMIT 1`, id).Scan(&svc, &st, &bbid)
			notify.EnqueueTx(tx, ctx, orgID, "BOOKING_CONFIRMED", []string{"WEB", "EMAIL"},
				notify.RecipientOf(ctx, h.Store, x.UserID), "", "",
				map[string]interface{}{"booking_id": bbid, "service_name": svc, "start_datetime": st}, "booking", &id, 5)
		}
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "status failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Slots возвращает свободные интервалы ресурса на дату.
func (h *Booking) Slots(c echo.Context) error {
	rid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	date := c.QueryParam("date")
	durMin, _ := strconv.Atoi(c.QueryParam("duration"))
	if date == "" || durMin <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "date/duration required"})
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad date"})
	}
	var slots []map[string]string
	err = h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		// Рабочий интервал дня.
		var ws, we string
		var isW *bool
		if err := tx.QueryRow(ctx, `
			SELECT is_working, start_time::text, end_time::text
			FROM service_resource_schedule_exception WHERE resource_id=$1 AND exception_date=$2::date`,
			rid, date).Scan(&isW, &ws, &we); err == nil {
			if !*isW {
				return nil
			}
		} else {
			dow := int(day.Weekday())
			if dow == 0 {
				dow = 7
			}
			var active bool
			if err := tx.QueryRow(ctx, `
				SELECT start_time::text, end_time::text, is_active
				FROM service_resource_schedule WHERE resource_id=$1 AND day_of_week=$2`, rid, dow).
				Scan(&ws, &we, &active); err != nil || !active {
				return nil
			}
		}
		wst, _ := time.Parse("15:04", ws[:5])
		wet, _ := time.Parse("15:04", we[:5])
		// Занятые интервалы.
		rows, err := tx.Query(ctx, `
			SELECT b.start_datetime, b.end_datetime FROM service_booking b
			JOIN service_booking_resource br ON br.booking_id=b.id
			WHERE br.resource_id=$1 AND b.start_datetime::date=$2::date
			  AND b.status_code NOT IN ('COMPLETED','CANCELED','NO_SHOW')`, rid, date)
		if err != nil {
			return err
		}
		type iv struct{ s, e time.Time }
		var busy []iv
		for rows.Next() {
			var s, e time.Time
			_ = rows.Scan(&s, &e)
			busy = append(busy, iv{s, e})
		}
		rows.Close()
		step := 15 * time.Minute
		dur := time.Duration(durMin) * time.Minute
		base := time.Date(day.Year(), day.Month(), day.Day(), wst.Hour(), wst.Minute(), 0, 0, time.Local)
		endDay := time.Date(day.Year(), day.Month(), day.Day(), wet.Hour(), wet.Minute(), 0, 0, time.Local)
		for t := base; !t.Add(dur).After(endDay); t = t.Add(step) {
			te := t.Add(dur)
			free := true
			for _, b := range busy {
				if t.Before(b.e) && te.After(b.s) {
					free = false
					break
				}
			}
			if free {
				slots = append(slots, map[string]string{"start": t.Format(time.RFC3339), "end": te.Format(time.RFC3339)})
			}
		}
		return nil
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "slots failed"})
	}
	if slots == nil {
		slots = []map[string]string{}
	}
	return c.JSON(http.StatusOK, slots)
}

// LinkReceipt привязывает позицию чека к бронированию (оплата услуги).
func (h *Booking) LinkReceipt(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		ReceiptItemID int64 `json:"receipt_item_id"`
	}
	if err := c.Bind(&b); err != nil || b.ReceiptItemID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "receipt_item_id required"})
	}
	res, err := h.Store.PG.Exec(c.Request().Context(), `
		UPDATE service_booking SET sales_receipt_item_id=$2 WHERE id=$1
		AND EXISTS(SELECT 1 FROM sales_receipt_item WHERE id=$2)`, id, b.ReceiptItemID)
	if err != nil || res.RowsAffected() == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "link failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Product-resource links.
func (h *Booking) LinkProductResource(c echo.Context) error {
	pid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		ResourceID  int64 `json:"resource_id"`
		IsMandatory bool  `json:"is_mandatory"`
	}
	if err := c.Bind(&b); err != nil || b.ResourceID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "resource_id required"})
	}
	_, err := h.Store.PG.Exec(c.Request().Context(), `
		INSERT INTO service_product_resource(product_id, resource_id, is_mandatory)
		VALUES($1,$2,$3) ON CONFLICT (product_id, resource_id)
		DO UPDATE SET is_mandatory=$3`, pid, b.ResourceID, b.IsMandatory)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "link failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Booking) ListProductResources(c echo.Context) error {
	pid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT r.id, r.name, r.resource_type_code, pr.is_mandatory
		FROM service_product_resource pr JOIN service_resource r ON r.id=pr.resource_id
		WHERE pr.product_id=$1`, pid)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, typ string
		var mand bool
		_ = rows.Scan(&id, &name, &typ, &mand)
		out = append(out, map[string]interface{}{"id": id, "name": name, "type": typ, "mandatory": mand})
	}
	return c.JSON(http.StatusOK, out)
}
