package service

import (
	"context"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/delivery"
	"retail-backend/internal/model"
	"retail-backend/internal/provider"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// DeliveryService — зоны, курьеры, доставки.
type DeliveryService struct {
	Store      *store.Store
	Reg        *provider.Registry
	IntRepo    repository.IntegrationRepo
	Zones      repository.ZoneRepo
	Couriers   repository.CourierRepo
	Deliveries repository.DeliveryRepo
	Notify     repository.NotifyRepo
	Audit      repository.AuditRepo
}

var deliveryTransitions = map[string][]string{
	"NEW":        {"ASSIGNED", "CANCELED"},
	"ASSIGNED":   {"PICKED_UP", "CANCELED"},
	"PICKED_UP":  {"IN_TRANSIT", "CANCELED"},
	"IN_TRANSIT": {"ARRIVED", "DELIVERED", "RETURNED"},
	"ARRIVED":    {"DELIVERED", "RETURNED"},
}

// externalType — типы, требующие внешней службы (трекинг).
func externalType(t string) bool {
	switch t {
	case "POST", "CDEK", "BOXBERY", "YANDEX_GO":
		return true
	}
	return false
}

// --- Zones ---

type CreateZoneInput struct {
	OrgID    int64    `json:"org_id"`
	Name     string   `json:"name"`
	Base     float64  `json:"base_price"`
	PerKg    *float64 `json:"price_per_kg"`
	FreeFrom *float64 `json:"free_delivery_from"`
	EtaMin   *int     `json:"estimated_days_min"`
	EtaMax   *int     `json:"estimated_days_max"`
}

func (s *DeliveryService) ListZones(ctx context.Context, orgID int64) []model.DeliveryZone {
	return s.Zones.List(ctx, s.Store.PG, orgID)
}

func (s *DeliveryService) CreateZone(ctx context.Context, in CreateZoneInput) (int64, error) {
	if in.OrgID == 0 || in.Name == "" || in.Base < 0 {
		return 0, BadRequest("org_id/name/base_price>=0 required")
	}
	id, err := s.Zones.Create(ctx, s.Store.PG, in.OrgID, in.Name, in.Base, in.PerKg, in.FreeFrom, in.EtaMin, in.EtaMax)
	if err != nil {
		return 0, Conflict("duplicate zone name")
	}
	return id, nil
}

// --- Couriers ---

type CreateCourierInput struct {
	OrgID     int64   `json:"org_id"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Phone     string  `json:"phone"`
	Vehicle   string  `json:"vehicle_type"`
	UserID    *int64  `json:"user_id"`
	ZoneIDs   []int64 `json:"assigned_zone_ids"`
}

func (s *DeliveryService) ListCouriers(ctx context.Context, orgID int64) []model.Courier {
	return s.Couriers.List(ctx, s.Store.PG, orgID)
}

func (s *DeliveryService) CreateCourier(ctx context.Context, in CreateCourierInput) (int64, error) {
	if in.OrgID == 0 || in.FirstName == "" || in.LastName == "" || in.Phone == "" {
		return 0, BadRequest("org_id/first_name/last_name/phone required")
	}
	id, err := s.Couriers.Create(ctx, s.Store.PG, in.OrgID, in.FirstName, in.LastName, in.Phone, in.Vehicle, in.UserID, in.ZoneIDs)
	if err != nil {
		return 0, BadRequest("create courier failed")
	}
	return id, nil
}

type CourierScheduleInput struct {
	Date    string `json:"date"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Working bool   `json:"is_working"`
}

func (s *DeliveryService) SetCourierSchedule(ctx context.Context, courierID int64, in CourierScheduleInput) error {
	if in.Date == "" || in.Start == "" || in.End == "" {
		return BadRequest("date/start/end required")
	}
	if err := s.Couriers.SetSchedule(ctx, s.Store.PG, courierID, in.Date, in.Start, in.End, in.Working); err != nil {
		return BadRequest("save failed")
	}
	return nil
}

// --- Deliveries ---

type CreateDeliveryInput struct {
	OrgID       int64    `json:"org_id"`
	SalesOrder  *int64   `json:"sales_order_id"`
	Type        string   `json:"delivery_type"`
	Address     string   `json:"address"`
	Recipient   string   `json:"recipient_name"`
	Phone       string   `json:"recipient_phone"`
	Email       string   `json:"email"`
	ZoneID      *int64   `json:"delivery_zone_id"`
	DesiredDate *string  `json:"desired_delivery_date"`
	Price       *float64 `json:"price"`
	Weight      float64  `json:"weight"`
}

var deliveryTypes = map[string]bool{
	"PICKUP": true, "COURIER": true, "POST": true, "CDEK": true,
	"BOXBERY": true, "YANDEX_GO": true, "OTHER": true,
}

func (s *DeliveryService) Create(ctx context.Context, in CreateDeliveryInput, userID int64, ip, ua string) (int64, error) {
	if !deliveryTypes[in.Type] {
		return 0, BadRequest("bad delivery_type")
	}
	var id int64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		org := in.OrgID
		address, rname, rphone, remail := in.Address, in.Recipient, in.Phone, in.Email
		var orderTotal float64
		if in.SalesOrder != nil {
			info, err := s.Deliveries.SalesOrderInfo(ctx, tx, *in.SalesOrder)
			if err != nil {
				return NotFound("no sales order")
			}
			org = info.OrgID
			if address == "" && info.Address != nil {
				address = *info.Address
			}
			if rname == "" {
				rname = info.Buyer
			}
			if rphone == "" {
				rphone = info.Phone
			}
			if remail == "" && info.Email != nil {
				remail = *info.Email
			}
			orderTotal = info.Total
		}
		if org == 0 {
			return BadRequest("org_id required")
		}
		if address == "" || rname == "" || rphone == "" {
			return BadRequest("address/recipient_name/recipient_phone required")
		}
		// Цена: явная → зона (вес, бесплатный порог).
		price := 0.0
		if in.Price != nil {
			price = *in.Price
		} else if in.ZoneID != nil {
			z, err := s.Zones.Get(ctx, tx, *in.ZoneID)
			if err != nil {
				return BadRequest("bad zone")
			}
			price = z.BasePrice
			if in.Weight > 0 && z.PricePerKg != nil {
				price += *z.PricePerKg * in.Weight
			}
			if z.FreeFrom != nil && orderTotal >= *z.FreeFrom {
				price = 0
			}
		}
		var tracking *string
		if externalType(in.Type) {
			if t, ok := s.externalTracking(ctx, tx, org); ok {
				tracking = &t
			}
		}
		var err error
		id, err = s.Deliveries.Create(ctx, tx, org, in.SalesOrder, in.Type, address,
			rname, rphone, remail, in.ZoneID, in.DesiredDate, model.Round2(price), in.Weight, tracking, userID)
		if err != nil {
			return Conflict("create delivery failed")
		}
		s.Deliveries.History(ctx, tx, id, "NEW", userID, "")
		s.Notify.EnqueueTx(ctx, tx, org, "DELIVERY_CREATED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, userID), "", "",
			map[string]interface{}{"delivery_id": id, "delivery_type": in.Type, "address": address}, "delivery", &id, 5)
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, se
		}
		return 0, Conflict("create delivery failed")
	}
	s.Audit.Log(ctx, s.Store.PG, &userID, "delivery.create", "Создание доставки", "delivery", &id, in, ip, ua, true, "")
	return id, nil
}

// externalTracking запрашивает трек у активной внешней службы (эмулятор по умолчанию).
func (s *DeliveryService) externalTracking(ctx context.Context, db repository.DBTX, org int64) (string, bool) {
	statuses := s.IntRepo.Statuses(ctx, db, s.Reg, org)
	if code := s.Reg.ActiveFor("DELIVERY", statuses); code != "" {
		// Пока только эмулятор; CDEK активируется договором (этап 14).
		t, err := (delivery.Emulator{}).CreateShipment(ctx, nil, 0, "")
		if err == nil {
			return t, true
		}
	}
	return "", false
}

func (s *DeliveryService) List(ctx context.Context, orgID int64, status string) []model.DeliveryOrder {
	return s.Deliveries.List(ctx, s.Store.PG, orgID, status)
}

func (s *DeliveryService) Get(ctx context.Context, id int64) (model.DeliveryDetail, error) {
	d, err := s.Deliveries.Get(ctx, s.Store.PG, id)
	if err != nil {
		return d, NotFound("no delivery")
	}
	return d, nil
}

// Assign назначает курьера (только из NEW).
func (s *DeliveryService) Assign(ctx context.Context, id, courierID, by int64, ip, ua string) error {
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		var cur string
		var org int64
		var zone *int64
		if err := tx.QueryRow(ctx, `SELECT status_code, organization_id, delivery_zone_id
			FROM delivery_order WHERE id=$1 FOR UPDATE`, id).Scan(&cur, &org, &zone); err != nil {
			return NotFound("no delivery")
		}
		if cur != "NEW" {
			return Conflict("assign only from NEW")
		}
		corg, active, avail, zones, err := s.Couriers.ForAssign(ctx, tx, courierID)
		if err != nil || corg != org {
			return BadRequest("bad courier")
		}
		if !active || !avail {
			return Conflict("courier unavailable")
		}
		if zone != nil && len(zones) > 0 && !containsZone(zones, *zone) {
			return Conflict("courier not assigned to zone")
		}
		if _, err := s.Deliveries.Assign(ctx, tx, id, courierID, by); err != nil {
			return Conflict("assign failed")
		}
		s.Deliveries.SetStatus(ctx, tx, id, "ASSIGNED")
		s.Deliveries.History(ctx, tx, id, "ASSIGNED", by, "")
		var cname, cphone string
		_ = tx.QueryRow(ctx, `SELECT first_name || ' ' || last_name, phone FROM delivery_courier WHERE id=$1`,
			courierID).Scan(&cname, &cphone)
		s.Notify.EnqueueTx(ctx, tx, org, "DELIVERY_ASSIGNED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, by), "", "",
			map[string]interface{}{"delivery_id": id, "courier_name": cname, "courier_phone": cphone},
			"delivery", &id, 5)
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return se
		}
		return Conflict("assign failed")
	}
	s.Audit.Log(ctx, s.Store.PG, &by, "delivery.assign", "Назначение курьера", "delivery", &id,
		map[string]interface{}{"courier_id": courierID}, ip, ua, true, "")
	return nil
}

func containsZone(zones []int64, zone int64) bool {
	for _, z := range zones {
		if z == zone {
			return true
		}
	}
	return false
}

// AcceptReject — курьер принимает/отклоняет назначение.
func (s *DeliveryService) Accept(ctx context.Context, id int64, accept bool, userID int64) error {
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		aid, _, st, err := s.Deliveries.ActiveAssignment(ctx, tx, id)
		if err != nil {
			return NotFound("no active assignment")
		}
		if st != "ASSIGNED" {
			return Conflict("assignment not pending")
		}
		if accept {
			if err := s.Deliveries.SetAssignment(ctx, tx, aid, "ACCEPTED"); err != nil {
				return Conflict("accept failed")
			}
			s.Deliveries.History(ctx, tx, id, "ASSIGNED", userID, "Курьер принял")
		} else {
			if err := s.Deliveries.SetAssignment(ctx, tx, aid, "REJECTED"); err != nil {
				return Conflict("accept failed")
			}
			s.Deliveries.SetStatus(ctx, tx, id, "NEW")
			s.Deliveries.History(ctx, tx, id, "NEW", userID, "Курьер отклонил, требуется переназначение")
		}
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return se
		}
		return Conflict("accept failed")
	}
	return nil
}

// SetStatus меняет статус доставки по карте переходов.
func (s *DeliveryService) SetStatus(ctx context.Context, id int64, status, comment, tracking string, userID int64) error {
	if status == "" {
		return BadRequest("status required")
	}
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		var cur string
		var org int64
		if err := tx.QueryRow(ctx, `SELECT status_code, organization_id FROM delivery_order WHERE id=$1 FOR UPDATE`, id).
			Scan(&cur, &org); err != nil {
			return NotFound("no delivery")
		}
		ok := false
		for _, t := range deliveryTransitions[cur] {
			if t == status {
				ok = true
			}
		}
		if !ok {
			return BadRequest("bad transition " + cur + " -> " + status)
		}
		s.Deliveries.SetStatus(ctx, tx, id, status)
		if tracking != "" {
			_, _ = tx.Exec(ctx, `UPDATE delivery_order SET tracking_number=$2 WHERE id=$1`, id, tracking)
		}
		s.Deliveries.History(ctx, tx, id, status, userID, comment)
		if status == "DELIVERED" {
			if aid, _, _, err := s.Deliveries.ActiveAssignment(ctx, tx, id); err == nil {
				if err := s.Deliveries.SetAssignment(ctx, tx, aid, "COMPLETED"); err != nil {
					return Conflict("status failed")
				}
			}
		}
		if status == "CANCELED" || status == "RETURNED" {
			if aid, _, _, err := s.Deliveries.ActiveAssignment(ctx, tx, id); err == nil {
				if err := s.Deliveries.SetAssignment(ctx, tx, aid, "CANCELED"); err != nil {
					return Conflict("status failed")
				}
			}
		}
		s.Notify.EnqueueTx(ctx, tx, org, "DELIVERY_STATUS_CHANGED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, userID), "", "",
			map[string]interface{}{"delivery_id": id, "new_status": status}, "delivery", &id, 5)
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
