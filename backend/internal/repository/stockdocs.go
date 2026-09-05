package repository

import (
	"context"
	"strconv"

	"retail-backend/internal/model"
)

// CounterpartyRepo — контрагенты.
type CounterpartyRepo struct{}

func (CounterpartyRepo) List(ctx context.Context, db DBTX, orgID int64, role string) []model.Counterparty {
	q := `SELECT id, inn, full_name, phone, is_supplier, is_buyer, credit_limit FROM counterparty WHERE organization_id=$1 AND is_active`
	args := []interface{}{orgID}
	if role == "supplier" {
		q += ` AND is_supplier=TRUE`
	} else if role == "buyer" {
		q += ` AND is_buyer=TRUE`
	}
	q += ` ORDER BY id`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Counterparty
	for rows.Next() {
		var cp model.Counterparty
		_ = rows.Scan(&cp.ID, &cp.INN, &cp.FullName, &cp.Phone, &cp.IsSupplier, &cp.IsBuyer, &cp.CreditLimit)
		out = append(out, cp)
	}
	if out == nil {
		out = []model.Counterparty{}
	}
	return out
}

func (CounterpartyRepo) Create(ctx context.Context, db DBTX, orgID int64, inn, fullName, phone string, sup, buy bool, limit float64) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO counterparty(organization_id, inn, full_name, phone, is_supplier, is_buyer, credit_limit)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7) RETURNING id`,
		orgID, inn, fullName, phone, sup, buy, limit).Scan(&id)
	return id, err
}

// WarehouseRepo — склады.
type WarehouseRepo struct{}

func (WarehouseRepo) List(ctx context.Context, db DBTX, orgID int64) []model.Warehouse {
	rows, err := db.Query(ctx, `
		SELECT id, code, name, warehouse_type FROM warehouse
		WHERE organization_id=$1 AND is_active ORDER BY id`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Warehouse
	for rows.Next() {
		var w model.Warehouse
		_ = rows.Scan(&w.ID, &w.Code, &w.Name, &w.Type)
		out = append(out, w)
	}
	if out == nil {
		out = []model.Warehouse{}
	}
	return out
}

func (WarehouseRepo) Create(ctx context.Context, db DBTX, orgID int64, code, name, address, wtype string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO warehouse(organization_id, code, name, address, warehouse_type)
		VALUES($1,$2,$3,$4,$5) RETURNING id`, orgID, code, name, address, wtype).Scan(&id)
	return id, err
}

func (WarehouseRepo) OrgOf(ctx context.Context, db DBTX, warehouseID int64) (int64, error) {
	var org int64
	err := db.QueryRow(ctx, `SELECT organization_id FROM warehouse WHERE id=$1`, warehouseID).Scan(&org)
	return org, err
}

// StockDocRepo — поступления и остатки-чтение.
type StockDocRepo struct{}

func (StockDocRepo) Balances(ctx context.Context, db DBTX, warehouseID int64) []model.Balance {
	rows, err := db.Query(ctx, `
		SELECT b.product_id, p.sku, p.name, b.quantity, b.reserved_quantity,
		       (b.quantity - b.reserved_quantity) AS available
		FROM warehouse_balance b JOIN catalog_product p ON p.id=b.product_id
		WHERE b.warehouse_id=$1 ORDER BY p.name`, warehouseID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Balance
	for rows.Next() {
		var b model.Balance
		_ = rows.Scan(&b.ProductID, &b.SKU, &b.Name, &b.Quantity, &b.Reserved, &b.Available)
		out = append(out, b)
	}
	if out == nil {
		out = []model.Balance{}
	}
	return out
}

// ReceiptLineInput — строка поступления.
type ReceiptLineInput struct {
	ProductID int64
	Quantity  float64
	Price     float64
	VATRate   float64
}

func (StockDocRepo) CreateReceipt(ctx context.Context, db DBTX, org, supplier, warehouse int64,
	number string, total, vat float64, userID int64, comment string, lines []ReceiptLineInput) (int64, error) {
	var id int64
	if err := db.QueryRow(ctx, `
		INSERT INTO receipt_document(organization_id, supplier_id, warehouse_id, document_number,
			total_amount, total_vat, responsible_id, comment)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')) RETURNING id`,
		org, supplier, warehouse, number, total, vat, userID, comment).Scan(&id); err != nil {
		return 0, err
	}
	for _, l := range lines {
		if _, err := db.Exec(ctx, `
			INSERT INTO receipt_line(document_id, product_id, quantity, price, vat_rate)
			VALUES($1,$2,$3,$4,$5)`, id, l.ProductID, l.Quantity, l.Price, l.VATRate); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// ReceiptForPost лочит документ и возвращает склад, организацию, номер и строки.
func (StockDocRepo) ReceiptForPost(ctx context.Context, db DBTX, id int64) (wh, org int64, number, whName string, items map[int64]float64, totalQty float64, err error) {
	var posted bool
	if err = db.QueryRow(ctx, `
		SELECT d.warehouse_id, d.is_posted, d.organization_id, d.document_number, w.name
		FROM receipt_document d JOIN warehouse w ON w.id=d.warehouse_id
		WHERE d.id=$1 FOR UPDATE OF d`, id).
		Scan(&wh, &posted, &org, &number, &whName); err != nil {
		return 0, 0, "", "", nil, 0, err
	}
	if posted {
		return 0, 0, "", "", nil, 0, ErrAlreadyPosted
	}
	rows, err := db.Query(ctx, `SELECT product_id, quantity FROM receipt_line WHERE document_id=$1`, id)
	if err != nil {
		return 0, 0, "", "", nil, 0, err
	}
	defer rows.Close()
	items = map[int64]float64{}
	for rows.Next() {
		var pid int64
		var qty float64
		_ = rows.Scan(&pid, &qty)
		items[pid] += qty
		totalQty += qty
	}
	return wh, org, number, whName, items, totalQty, nil
}

func (StockDocRepo) MarkPosted(ctx context.Context, db DBTX, id int64) {
	_, _ = db.Exec(ctx, `UPDATE receipt_document SET is_posted=TRUE, posted_at=NOW() WHERE id=$1`, id)
}

func (StockDocRepo) ListReceipts(ctx context.Context, db DBTX, warehouseID int64) []model.StockReceipt {
	q := `SELECT id, document_number, document_date::text, total_amount, is_posted FROM receipt_document`
	var args []interface{}
	if warehouseID != 0 {
		q += ` WHERE warehouse_id=$1`
		args = append(args, warehouseID)
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.StockReceipt
	for rows.Next() {
		var d model.StockReceipt
		_ = rows.Scan(&d.ID, &d.Number, &d.Date, &d.Total, &d.Posted)
		out = append(out, d)
	}
	if out == nil {
		out = []model.StockReceipt{}
	}
	return out
}

func (StockDocRepo) NextDocNumber(ctx context.Context, db DBTX, table string, orgID int64, prefix string) string {
	var n int64
	_ = db.QueryRow(ctx, `SELECT COUNT(*)+1 FROM `+table+` WHERE organization_id=$1`, orgID).Scan(&n)
	return prefix + strconv.FormatInt(n, 10)
}
