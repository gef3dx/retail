package repository

import (
	"context"

	"retail-backend/internal/model"
)

// OrderRepo — заказы покупателей.
type OrderRepo struct{}

// OrderLineData — строка заказа для чтения/проверок.
type OrderLineData struct {
	ID       int64
	Product  int64
	Quantity float64
	Price    float64
	VAT      float64
}

func (OrderRepo) Create(ctx context.Context, db DBTX, org, buyer, warehouse int64, number, otype string,
	total, vat float64, manager int64, lines []OrderLineData) (int64, error) {
	var id int64
	if err := db.QueryRow(ctx, `
		INSERT INTO sales_order(organization_id, buyer_id, warehouse_id, order_number, order_type,
			total_amount, total_vat, manager_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		org, buyer, warehouse, number, otype, total, vat, manager).Scan(&id); err != nil {
		return 0, err
	}
	for _, l := range lines {
		if _, err := db.Exec(ctx, `
			INSERT INTO sales_order_line(order_id, product_id, quantity, price, vat_rate)
			VALUES($1,$2,$3,$4,$5)`, id, l.Product, l.Quantity, l.Price, l.VAT); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (OrderRepo) List(ctx context.Context, db DBTX, status string) []model.Order {
	q := `SELECT o.id, o.order_number, o.order_type, o.total_amount, o.status, cp.full_name
		FROM sales_order o JOIN counterparty cp ON cp.id=o.buyer_id`
	var args []interface{}
	if status != "" {
		q += ` WHERE o.status=$1`
		args = append(args, status)
	}
	q += ` ORDER BY o.id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Order
	for rows.Next() {
		var o model.Order
		_ = rows.Scan(&o.ID, &o.Number, &o.Type, &o.Total, &o.Status, &o.Buyer)
		out = append(out, o)
	}
	if out == nil {
		out = []model.Order{}
	}
	return out
}

// OrderHead — шапка заказа с блокировкой.
type OrderHead struct {
	ID        int64
	Org       int64
	Warehouse int64
	Buyer     int64
	Manager   int64
	Status    string
	Number    string
}

func (OrderRepo) HeadForUpdate(ctx context.Context, db DBTX, id int64) (OrderHead, error) {
	var h OrderHead
	h.ID = id
	err := db.QueryRow(ctx, `
		SELECT organization_id, warehouse_id, buyer_id, manager_id, status, order_number
		FROM sales_order WHERE id=$1 FOR UPDATE`, id).
		Scan(&h.Org, &h.Warehouse, &h.Buyer, &h.Manager, &h.Status, &h.Number)
	return h, err
}

func (OrderRepo) Lines(ctx context.Context, db DBTX, orderID int64) []OrderLineData {
	rows, err := db.Query(ctx, `SELECT id, product_id, quantity, price, vat_rate FROM sales_order_line WHERE order_id=$1`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []OrderLineData
	for rows.Next() {
		var l OrderLineData
		_ = rows.Scan(&l.ID, &l.Product, &l.Quantity, &l.Price, &l.VAT)
		out = append(out, l)
	}
	return out
}

func (OrderRepo) LinesReserved(ctx context.Context, db DBTX, orderID int64) map[int64]float64 {
	rows, err := db.Query(ctx, `SELECT product_id, reserved_quantity FROM sales_order_line WHERE order_id=$1`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[int64]float64{}
	for rows.Next() {
		var pid int64
		var qty float64
		_ = rows.Scan(&pid, &qty)
		out[pid] += qty
	}
	return out
}

func (OrderRepo) SetLineReserved(ctx context.Context, db DBTX, lineID int64, qty float64) {
	_, _ = db.Exec(ctx, `UPDATE sales_order_line SET reserved_quantity=$2 WHERE id=$1`, lineID, qty)
}

func (OrderRepo) SetStatus(ctx context.Context, db DBTX, orderID int64, status string) {
	_, _ = db.Exec(ctx, `UPDATE sales_order SET status=$2 WHERE id=$1`, orderID, status)
}

func (OrderRepo) Detail(ctx context.Context, db DBTX, id int64) (model.OrderDetail, error) {
	var d model.OrderDetail
	if err := db.QueryRow(ctx, `
		SELECT order_number, status, total_amount, warehouse_id FROM sales_order WHERE id=$1`, id).
		Scan(&d.Number, &d.Status, &d.Total, &d.WarehouseID); err != nil {
		return d, err
	}
	d.ID = id
	rows, err := db.Query(ctx, `
		SELECT l.id, l.product_id, p.name, l.quantity, l.price, l.reserved_quantity,
		       COALESCE((SELECT SUM(s.quantity) FROM shipment_line s
		                 JOIN shipment_document d ON d.id=s.document_id
		                 WHERE s.order_line_id=l.id AND d.is_posted),0) AS shipped
		FROM sales_order_line l JOIN catalog_product p ON p.id=l.product_id
		WHERE l.order_id=$1 ORDER BY l.id`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var l model.OrderLine
		_ = rows.Scan(&l.ID, &l.ProductID, &l.Name, &l.Quantity, &l.Price, &l.Reserved, &l.Shipped)
		d.Lines = append(d.Lines, l)
	}
	return d, nil
}

func (OrderRepo) RemainingTotal(ctx context.Context, db DBTX, orderID int64) float64 {
	var left float64
	_ = db.QueryRow(ctx, `
		SELECT COALESCE(SUM(l.quantity - COALESCE((SELECT SUM(s.quantity) FROM shipment_line s
			JOIN shipment_document d ON d.id=s.document_id
			WHERE s.order_line_id=l.id AND d.is_posted),0)),0)
		FROM sales_order_line l WHERE l.order_id=$1`, orderID).Scan(&left)
	return left
}

// ShipmentRepo — отгрузки.
type ShipmentRepo struct{}

func (ShipmentRepo) Create(ctx context.Context, db DBTX, org, buyer, warehouse int64, orderID *int64,
	number string, total, vat float64, userID int64) (int64, error) {
	var sid int64
	err := db.QueryRow(ctx, `
		INSERT INTO shipment_document(organization_id, buyer_id, warehouse_id, sales_order_id,
			document_number, total_amount, total_vat, is_posted, posted_at, responsible_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,TRUE,NOW(),$8) RETURNING id`,
		org, buyer, warehouse, orderID, number, total, vat, userID).Scan(&sid)
	return sid, err
}

func (ShipmentRepo) AddLine(ctx context.Context, db DBTX, docID, productID int64, orderLineID *int64, qty, price, vat float64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO shipment_line(document_id, product_id, order_line_id, quantity, price, vat_rate)
		VALUES($1,$2,$3,$4,$5,$6)`, docID, productID, orderLineID, qty, price, vat)
	return err
}

func (ShipmentRepo) List(ctx context.Context, db DBTX, orderID int64) []model.Shipment {
	q := `SELECT id, document_number, total_amount, is_posted, sales_order_id FROM shipment_document`
	var args []interface{}
	if orderID != 0 {
		q += ` WHERE sales_order_id=$1`
		args = append(args, orderID)
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Shipment
	for rows.Next() {
		var s model.Shipment
		_ = rows.Scan(&s.ID, &s.Number, &s.Total, &s.Posted, &s.OrderID)
		out = append(out, s)
	}
	if out == nil {
		out = []model.Shipment{}
	}
	return out
}
