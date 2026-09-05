package repository

import (
	"context"

	"retail-backend/internal/model"
)

// ZoneRepo — зоны доставки.
type ZoneRepo struct{}

func (ZoneRepo) List(ctx context.Context, db DBTX, orgID int64) []model.DeliveryZone {
	rows, err := db.Query(ctx, `
		SELECT id, name, base_price, price_per_kg, free_delivery_from,
		       estimated_days_min, estimated_days_max, is_active
		FROM delivery_zone WHERE organization_id=$1 ORDER BY id`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.DeliveryZone
	for rows.Next() {
		var z model.DeliveryZone
		_ = rows.Scan(&z.ID, &z.Name, &z.BasePrice, &z.PricePerKg, &z.FreeFrom,
			&z.EstimatedMin, &z.EstimatedMax, &z.IsActive)
		out = append(out, z)
	}
	if out == nil {
		out = []model.DeliveryZone{}
	}
	return out
}

func (ZoneRepo) Create(ctx context.Context, db DBTX, orgID int64, name string, base float64,
	perKg, freeFrom *float64, etaMin, etaMax *int) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO delivery_zone(organization_id, name, base_price, price_per_kg,
			free_delivery_from, estimated_days_min, estimated_days_max)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		orgID, name, base, perKg, freeFrom, etaMin, etaMax).Scan(&id)
	return id, err
}

func (ZoneRepo) Get(ctx context.Context, db DBTX, id int64) (model.DeliveryZone, error) {
	var z model.DeliveryZone
	err := db.QueryRow(ctx, `
		SELECT id, name, base_price, price_per_kg, free_delivery_from,
		       estimated_days_min, estimated_days_max, is_active
		FROM delivery_zone WHERE id=$1`, id).
		Scan(&z.ID, &z.Name, &z.BasePrice, &z.PricePerKg, &z.FreeFrom,
			&z.EstimatedMin, &z.EstimatedMax, &z.IsActive)
	return z, err
}

// CourierRepo — курьеры и их график.
type CourierRepo struct{}

func (CourierRepo) List(ctx context.Context, db DBTX, orgID int64) []model.Courier {
	rows, err := db.Query(ctx, `
		SELECT id, first_name, last_name, phone, vehicle_type, assigned_zone_ids, is_active, is_available
		FROM delivery_courier WHERE organization_id=$1 ORDER BY id`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Courier
	for rows.Next() {
		var c model.Courier
		_ = rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Phone, &c.Vehicle, &c.ZoneIDs, &c.IsActive, &c.IsAvailable)
		out = append(out, c)
	}
	if out == nil {
		out = []model.Courier{}
	}
	return out
}

func (CourierRepo) Create(ctx context.Context, db DBTX, orgID int64, first, last, phone, vehicle string,
	userID *int64, zones []int64) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO delivery_courier(organization_id, user_id, first_name, last_name, phone,
			vehicle_type, assigned_zone_ids)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7) RETURNING id`,
		orgID, userID, first, last, phone, vehicle, zones).Scan(&id)
	return id, err
}

// ForAssign возвращает курьера с блокировкой для назначения.
func (CourierRepo) ForAssign(ctx context.Context, db DBTX, id int64) (orgID int64, active, avail bool, zones []int64, err error) {
	err = db.QueryRow(ctx, `
		SELECT organization_id, is_active, is_available, assigned_zone_ids
		FROM delivery_courier WHERE id=$1 FOR UPDATE`, id).Scan(&orgID, &active, &avail, &zones)
	return orgID, active, avail, zones, err
}

func (CourierRepo) SetSchedule(ctx context.Context, db DBTX, courierID int64, date, start, end string, working bool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO delivery_courier_schedule(courier_id, work_date, start_time, end_time, is_working)
		VALUES($1,$2::date,$3::time,$4::time,$5)
		ON CONFLICT (courier_id, work_date)
		DO UPDATE SET start_time=$3::time, end_time=$4::time, is_working=$5`,
		courierID, date, start, end, working)
	return err
}

// DeliveryRepo — доставки, назначения, история.
type DeliveryRepo struct{}

func (DeliveryRepo) Create(ctx context.Context, db DBTX, orgID int64, salesOrder *int64,
	dtype, address, rname, rphone, remail string, zoneID *int64, desiredDate *string,
	price, weight float64, tracking *string, createdBy int64) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO delivery_order(organization_id, sales_order_id, delivery_type_code,
			delivery_address, recipient_name, recipient_phone, recipient_email,
			delivery_zone_id, desired_delivery_date, delivery_price, total_weight,
			tracking_number, created_by_id)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8, NULLIF($9,'')::date,$10,NULLIF($11,0),$12,$13) RETURNING id`,
		orgID, salesOrder, dtype, address, rname, rphone, remail,
		zoneID, desiredDate, price, weight, tracking, createdBy).Scan(&id)
	return id, err
}

func (DeliveryRepo) List(ctx context.Context, db DBTX, orgID int64, status string) []model.DeliveryOrder {
	q := `SELECT d.id, d.delivery_type_code, d.delivery_address, d.recipient_name,
		d.delivery_price, d.tracking_number, d.status_code,
		(SELECT c.first_name || ' ' || c.last_name FROM delivery_order_assignment a
		 JOIN delivery_courier c ON c.id=a.courier_id
		 WHERE a.delivery_order_id=d.id AND a.status IN ('ASSIGNED','ACCEPTED') LIMIT 1),
		d.desired_delivery_date::text
		FROM delivery_order d WHERE d.organization_id=$1`
	args := []interface{}{orgID}
	if status != "" {
		args = append(args, status)
		q += ` AND d.status_code=$2`
	}
	q += ` ORDER BY d.id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.DeliveryOrder
	for rows.Next() {
		var d model.DeliveryOrder
		_ = rows.Scan(&d.ID, &d.Type, &d.Address, &d.Recipient, &d.Price, &d.Tracking, &d.Status, &d.Courier, &d.DesiredDate)
		out = append(out, d)
	}
	if out == nil {
		out = []model.DeliveryOrder{}
	}
	return out
}

func (DeliveryRepo) Get(ctx context.Context, db DBTX, id int64) (model.DeliveryDetail, error) {
	var d model.DeliveryDetail
	if err := db.QueryRow(ctx, `
		SELECT organization_id, sales_order_id, delivery_type_code, delivery_address,
		       recipient_name, recipient_phone, delivery_zone_id, delivery_price,
		       tracking_number, status_code, desired_delivery_date::text
		FROM delivery_order WHERE id=$1`, id).
		Scan(&d.OrgID, &d.SalesOrder, &d.Type, &d.Address, &d.Recipient, &d.Phone,
			&d.ZoneID, &d.Price, &d.Tracking, &d.Status, &d.DesiredDate); err != nil {
		return d, err
	}
	d.ID = id
	arows, err := db.Query(ctx, `
		SELECT a.id, a.courier_id, c.first_name || ' ' || c.last_name, a.status
		FROM delivery_order_assignment a JOIN delivery_courier c ON c.id=a.courier_id
		WHERE a.delivery_order_id=$1 ORDER BY a.id`, id)
	if err == nil {
		defer arows.Close()
		for arows.Next() {
			var a model.Assignment
			_ = arows.Scan(&a.ID, &a.CourierID, &a.Courier, &a.Status)
			d.Assignments = append(d.Assignments, a)
		}
	}
	hrows, err := db.Query(ctx, `
		SELECT status_code, changed_by_id, comment, changed_at::text
		FROM delivery_order_status_history WHERE delivery_order_id=$1 ORDER BY id`, id)
	if err == nil {
		defer hrows.Close()
		for hrows.Next() {
			var h model.DeliveryHistory
			_ = hrows.Scan(&h.Status, &h.By, &h.Comment, &h.At)
			d.History = append(d.History, h)
		}
	}
	if d.History == nil {
		d.History = []model.DeliveryHistory{}
	}
	return d, nil
}

func (DeliveryRepo) SetStatus(ctx context.Context, db DBTX, id int64, status string) {
	_, _ = db.Exec(ctx, `
		UPDATE delivery_order SET status_code=$2,
			completed_at = CASE WHEN $2::varchar IN ('DELIVERED','CANCELED','RETURNED') THEN NOW() ELSE completed_at END
		WHERE id=$1`, id, status)
}

func (DeliveryRepo) History(ctx context.Context, db DBTX, id int64, status string, by int64, comment string) {
	_, _ = db.Exec(ctx, `
		INSERT INTO delivery_order_status_history(delivery_order_id, status_code, changed_by_id, comment)
		VALUES($1,$2,$3,NULLIF($4,''))`, id, status, by, comment)
}

func (DeliveryRepo) Assign(ctx context.Context, db DBTX, orderID, courierID, by int64) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO delivery_order_assignment(delivery_order_id, courier_id, assigned_by_id)
		VALUES($1,$2,$3) RETURNING id`, orderID, courierID, by).Scan(&id)
	return id, err
}

func (DeliveryRepo) ActiveAssignment(ctx context.Context, db DBTX, orderID int64) (id, courierID int64, status string, err error) {
	err = db.QueryRow(ctx, `
		SELECT id, courier_id, status FROM delivery_order_assignment
		WHERE delivery_order_id=$1 AND status IN ('ASSIGNED','ACCEPTED') ORDER BY id DESC LIMIT 1`,
		orderID).Scan(&id, &courierID, &status)
	return id, courierID, status, err
}

func (DeliveryRepo) SetAssignment(ctx context.Context, db DBTX, id int64, status string) error {
	_, err := db.Exec(ctx, `
		UPDATE delivery_order_assignment SET status=$2,
			accepted_at = CASE WHEN $2::varchar='ACCEPTED' THEN NOW() ELSE accepted_at END,
			pickup_at = CASE WHEN $2::varchar='ACCEPTED' THEN COALESCE(pickup_at, NOW()) ELSE pickup_at END,
			delivered_at = CASE WHEN $2::varchar='COMPLETED' THEN NOW() ELSE delivered_at END
		WHERE id=$1`, id, status)
	return err
}

// SalesOrderInfo подтягивает данные заказа покупателя для автозаполнения.
type SalesOrderInfo struct {
	OrgID     int64
	Buyer     string
	Phone     string
	Email     *string
	Address   *string
	Total     float64
	Warehouse int64
}

func (DeliveryRepo) SalesOrderInfo(ctx context.Context, db DBTX, orderID int64) (SalesOrderInfo, error) {
	var s SalesOrderInfo
	err := db.QueryRow(ctx, `
		SELECT o.organization_id, cp.full_name, cp.phone, cp.email, o.delivery_address, o.total_amount, o.warehouse_id
		FROM sales_order o JOIN counterparty cp ON cp.id=o.buyer_id WHERE o.id=$1`, orderID).
		Scan(&s.OrgID, &s.Buyer, &s.Phone, &s.Email, &s.Address, &s.Total, &s.Warehouse)
	return s, err
}
