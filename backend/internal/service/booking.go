package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// BookingService — ресурсы, расписание, бронирования.
type BookingService struct {
	Store     *store.Store
	Resources repository.ResourceRepo
	Bookings  repository.BookingRepo
	Notify    repository.NotifyRepo
}

var bookingTransitions = map[string][]string{
	"PENDING":     {"CONFIRMED", "CANCELED"},
	"CONFIRMED":   {"IN_PROGRESS", "CANCELED", "NO_SHOW"},
	"IN_PROGRESS": {"COMPLETED", "NO_SHOW"},
}

// --- Resources ---

func (s *BookingService) ListResources(ctx context.Context, orgID int64) []model.Resource {
	return s.Resources.List(ctx, s.Store.PG, orgID)
}

type CreateResourceInput struct {
	OrgID    int64  `json:"org_id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	UserID   *int64 `json:"user_id"`
	Location string `json:"location"`
}

func (s *BookingService) CreateResource(ctx context.Context, in CreateResourceInput) (int64, error) {
	if in.OrgID == 0 || in.Type == "" || in.Name == "" {
		return 0, BadRequest("org_id/type/name required")
	}
	id, err := s.Resources.Create(ctx, s.Store.PG, in.OrgID, in.Type, in.Name, in.UserID, in.Location)
	if err != nil {
		return 0, BadRequest("create failed (bad type?)")
	}
	return id, nil
}

func (s *BookingService) GetSchedule(ctx context.Context, resourceID int64) []model.SchedDay {
	return s.Resources.Schedule(ctx, s.Store.PG, resourceID)
}

func (s *BookingService) PutSchedule(ctx context.Context, resourceID int64, days []model.SchedDay) error {
	if len(days) == 0 {
		return BadRequest("days required")
	}
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		for _, d := range days {
			if d.DOW < 1 || d.DOW > 7 || d.Start == "" || d.End == "" {
				return BadRequest("bad day")
			}
			if err := s.Resources.SaveScheduleDay(ctx, tx, resourceID, d); err != nil {
				return BadRequest("save failed")
			}
		}
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return se
		}
		return BadRequest("save failed")
	}
	return nil
}

type ScheduleExceptionInput struct {
	Date      string `json:"date"`
	IsWorking bool   `json:"is_working"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Reason    string `json:"reason"`
}

func (s *BookingService) AddException(ctx context.Context, resourceID int64, in ScheduleExceptionInput) error {
	if in.Date == "" {
		return BadRequest("date required")
	}
	if err := s.Resources.AddException(ctx, s.Store.PG, resourceID, in.Date, in.IsWorking, in.Start, in.End, in.Reason); err != nil {
		return BadRequest("save failed")
	}
	return nil
}

// within проверяет вхождение [start,end] в рабочие часы HH:MM.
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

// workingInterval возвращает рабочий интервал ресурса на дату + причину отказа.
func (s *BookingService) workingInterval(ctx context.Context, db repository.DBTX, resourceID int64, day time.Time) (string, string, bool, string) {
	date := day.Format("2006-01-02")
	if working, st, en, found := s.Resources.Exception(ctx, db, resourceID, date); found {
		if !working {
			return "", "", false, "day off (exception)"
		}
		if st != "" && en != "" {
			return st, en, true, ""
		}
		return "00:00", "23:59", true, ""
	}
	if !s.Resources.HasSchedule(ctx, db, resourceID) {
		return "00:00", "23:59", true, ""
	}
	dow := int(day.Weekday())
	if dow == 0 {
		dow = 7
	}
	st, en, active, found := s.Resources.DaySchedule(ctx, db, resourceID, dow)
	if !found || !active {
		return "", "", false, "no schedule for weekday"
	}
	return st, en, true, ""
}

func (s *BookingService) checkAvailability(ctx context.Context, db repository.DBTX, resourceID int64, start, end time.Time) error {
	ws, we, ok, reason := s.workingInterval(ctx, db, resourceID, start)
	if !ok {
		if reason == "" {
			reason = "off schedule"
		}
		return Conflict("resource off schedule: " + reason)
	}
	if !within(ws, we, start, end) {
		if _, _, _, found := s.Resources.Exception(ctx, db, resourceID, start.Format("2006-01-02")); found {
			return Conflict("resource off schedule: exception hours")
		}
		return Conflict("resource off schedule: no schedule for weekday")
	}
	busy, err := s.Resources.Overlaps(ctx, db, resourceID, start, end, 0)
	if err != nil {
		return Conflict("availability check failed")
	}
	if busy {
		return Conflict("resource busy")
	}
	return nil
}

// Slots возвращает свободные интервалы ресурса на дату.
func (s *BookingService) Slots(ctx context.Context, resourceID int64, date string, durMin int) ([]model.Slot, error) {
	if date == "" || durMin <= 0 {
		return nil, BadRequest("date/duration required")
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, BadRequest("bad date")
	}
	var slots []model.Slot
	err = s.Store.Tx(ctx, func(tx pgx.Tx) error {
		ws, we, ok, _ := s.workingInterval(ctx, tx, resourceID, day)
		if !ok {
			return nil
		}
		wst, _ := time.Parse("15:04", ws)
		wet, _ := time.Parse("15:04", we)
		busy := s.Resources.BusyIntervals(ctx, tx, resourceID, date)
		step := 15 * time.Minute
		dur := time.Duration(durMin) * time.Minute
		base := time.Date(day.Year(), day.Month(), day.Day(), wst.Hour(), wst.Minute(), 0, 0, time.Local)
		endDay := time.Date(day.Year(), day.Month(), day.Day(), wet.Hour(), wet.Minute(), 0, 0, time.Local)
		for t := base; !t.Add(dur).After(endDay); t = t.Add(step) {
			te := t.Add(dur)
			free := true
			for _, b := range busy {
				if t.Before(b[1]) && te.After(b[0]) {
					free = false
					break
				}
			}
			if free {
				slots = append(slots, model.Slot{Start: t.Format(time.RFC3339), End: te.Format(time.RFC3339)})
			}
		}
		return nil
	})
	if err != nil {
		return nil, Conflict("slots failed")
	}
	if slots == nil {
		slots = []model.Slot{}
	}
	return slots, nil
}

// --- Bookings ---

type CreateBookingInput struct {
	OrgID       int64    `json:"org_id"`
	ProductID   int64    `json:"product_id"`
	ResourceIDs []int64  `json:"resource_ids"`
	Start       string   `json:"start"`
	Duration    *int     `json:"duration_minutes"`
	CustomerID  *int64   `json:"customer_id"`
	CustName    string   `json:"customer_name"`
	CustPhone   string   `json:"customer_phone"`
	CustEmail   string   `json:"customer_email"`
	Notes       string   `json:"notes"`
	Price       *float64 `json:"price"`
}

func (s *BookingService) Create(ctx context.Context, in CreateBookingInput, userID int64) (int64, error) {
	if in.OrgID == 0 || in.ProductID == 0 || in.Start == "" {
		return 0, BadRequest("org_id/product_id/start required")
	}
	start, err := time.Parse(time.RFC3339, in.Start)
	if err != nil {
		return 0, BadRequest("start must be RFC3339")
	}
	if start.Before(time.Now().Add(-5 * time.Minute)) {
		return 0, BadRequest("start in the past")
	}
	var bid int64
	err = s.Store.Tx(ctx, func(tx pgx.Tx) error {
		ptype, pname, dur, _, enabled, basePrice, err := s.Bookings.ServiceForBooking(ctx, tx, in.ProductID)
		if err != nil {
			return NotFound("no product")
		}
		if ptype != "SERVICE" {
			return BadRequest("product is not a service")
		}
		if !enabled {
			return BadRequest("booking disabled for service")
		}
		duration := 0
		if in.Duration != nil {
			duration = *in.Duration
		} else if dur != nil {
			duration = *dur
		}
		if duration <= 0 {
			return BadRequest("duration required (product has none)")
		}
		end := start.Add(time.Duration(duration) * time.Minute)
		need := s.Resources.MandatoryResources(ctx, tx, in.ProductID)
		given := map[int64]bool{}
		for _, rid := range in.ResourceIDs {
			if !s.Resources.ActiveInOrg(ctx, tx, rid, in.OrgID) {
				return BadRequest("bad resource")
			}
			given[rid] = true
		}
		for _, rid := range need {
			if !given[rid] {
				return BadRequest("mandatory resource missing")
			}
		}
		for rid := range given {
			if err := s.checkAvailability(ctx, tx, rid, start, end); err != nil {
				return err
			}
		}
		price := 0.0
		if in.Price != nil {
			price = *in.Price
		} else if basePrice != nil {
			price = *basePrice
		}
		bid, err = s.Bookings.Create(ctx, tx, in.OrgID, in.CustomerID, in.CustName, in.CustPhone,
			in.CustEmail, start, end, duration, in.Notes, userID)
		if err != nil {
			return Conflict("create booking failed")
		}
		itemID, err := s.Bookings.AddItem(ctx, tx, bid, in.ProductID, price, duration)
		if err != nil {
			return Conflict("create booking failed")
		}
		for rid := range given {
			if err := s.Bookings.AttachResource(ctx, tx, bid, rid, itemID); err != nil {
				return Conflict("create booking failed")
			}
		}
		s.Bookings.History(ctx, tx, bid, "PENDING", userID, "Создано")
		s.Notify.EnqueueTx(ctx, tx, in.OrgID, "BOOKING_CREATED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, userID), "", "",
			map[string]interface{}{"booking_id": bid, "service_name": pname,
				"start_datetime": start.Format("2006-01-02 15:04")}, "booking", &bid, 5)
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, se
		}
		return 0, Conflict("create booking failed")
	}
	return bid, nil
}

func (s *BookingService) List(ctx context.Context, orgID int64, date, status string) []model.Booking {
	return s.Bookings.List(ctx, s.Store.PG, repository.BookingFilter{OrgID: orgID, Date: date, Status: status})
}

func (s *BookingService) Get(ctx context.Context, id int64) (model.BookingDetail, error) {
	d, err := s.Bookings.Get(ctx, s.Store.PG, id)
	if err != nil {
		return d, NotFound("no booking")
	}
	return d, nil
}

func (s *BookingService) SetStatus(ctx context.Context, id int64, status, comment string, userID int64) error {
	if status == "" {
		return BadRequest("status required")
	}
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		cur, orgID, err := s.Bookings.StatusOf(ctx, tx, id)
		if err != nil {
			return NotFound("no booking")
		}
		ok := false
		for _, t := range bookingTransitions[cur] {
			if t == status {
				ok = true
			}
		}
		if !ok {
			return BadRequest("bad transition " + cur + " -> " + status)
		}
		s.Bookings.SetStatus(ctx, tx, id, status)
		s.Bookings.History(ctx, tx, id, status, userID, comment)
		if status == "CONFIRMED" {
			svc, st := s.Bookings.ConfirmInfo(ctx, tx, id)
			s.Notify.EnqueueTx(ctx, tx, orgID, "BOOKING_CONFIRMED", []string{"WEB", "EMAIL"},
				s.Notify.RecipientOf(ctx, tx, userID), "", "",
				map[string]interface{}{"booking_id": id, "service_name": svc, "start_datetime": st}, "booking", &id, 5)
		}
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return se
		}
		return Conflict("status failed")
	}
	return nil
}

func (s *BookingService) LinkReceipt(ctx context.Context, bookingID, itemID int64) error {
	if itemID == 0 {
		return BadRequest("receipt_item_id required")
	}
	if err := s.Bookings.LinkReceipt(ctx, s.Store.PG, bookingID, itemID); err != nil {
		return BadRequest("link failed")
	}
	return nil
}

type LinkProductResourceInput struct {
	ResourceID  int64 `json:"resource_id"`
	IsMandatory bool  `json:"is_mandatory"`
}

func (s *BookingService) LinkProductResource(ctx context.Context, productID int64, in LinkProductResourceInput) error {
	if in.ResourceID == 0 {
		return BadRequest("resource_id required")
	}
	if err := s.Bookings.LinkProductResource(ctx, s.Store.PG, productID, in.ResourceID, in.IsMandatory); err != nil {
		return BadRequest("link failed")
	}
	return nil
}

func (s *BookingService) ListProductResources(ctx context.Context, productID int64) []model.ProductResource {
	return s.Bookings.ProductResources(ctx, s.Store.PG, productID)
}
