package repository

import (
	"context"
	"fmt"
	"strconv"

	"retail-backend/internal/model"
)

// RegisterRepo — кассы ККТ.
type RegisterRepo struct{}

func (RegisterRepo) List(ctx context.Context, db DBTX, orgID int64) []model.Register {
	q := `SELECT id, organization_id, reg_number, model, status, warehouse_id FROM cash_register`
	var args []interface{}
	if orgID != 0 {
		q += ` WHERE organization_id=$1`
		args = append(args, orgID)
	}
	q += ` ORDER BY id`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Register
	for rows.Next() {
		var r model.Register
		_ = rows.Scan(&r.ID, &r.OrganizationID, &r.RegNumber, &r.Model, &r.Status, &r.WarehouseID)
		out = append(out, r)
	}
	if out == nil {
		out = []model.Register{}
	}
	return out
}

func (RegisterRepo) Create(ctx context.Context, db DBTX, orgID int64, regNumber, modelName, address string) (int64, error) {
	if modelName == "" {
		modelName = "MOCK-KKT"
	}
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO cash_register(organization_id, reg_number, model, installation_address)
		VALUES($1,$2,$3,NULLIF($4,'')) RETURNING id`, orgID, regNumber, modelName, address).Scan(&id)
	if err != nil {
		return 0, err
	}
	_, _ = db.Exec(ctx, `
		INSERT INTO ofd_settings(organization_id) VALUES($1) ON CONFLICT (organization_id, provider) DO NOTHING`, orgID)
	return id, nil
}

// Patch привязывает склад (warehouseID==nil → отвязать, hasWh==false → не трогать) и статус.
func (RegisterRepo) Patch(ctx context.Context, db DBTX, id int64, warehouseID *int64, hasWh bool, status *string) error {
	if hasWh {
		if warehouseID == nil {
			_, _ = db.Exec(ctx, `UPDATE cash_register SET warehouse_id=NULL WHERE id=$1`, id)
		} else {
			res, err := db.Exec(ctx, `
				UPDATE cash_register r SET warehouse_id=$2 FROM warehouse w
				WHERE r.id=$1 AND w.id=$2 AND w.organization_id=r.organization_id`, id, *warehouseID)
			if err != nil || res.RowsAffected() == 0 {
				return fmt.Errorf("bad warehouse (other org?)")
			}
		}
	}
	if status != nil {
		_, _ = db.Exec(ctx, `UPDATE cash_register SET status=$2 WHERE id=$1`, id, *status)
	}
	return nil
}

// OrgWarehouse возвращает организацию и склад кассы.
func (RegisterRepo) OrgWarehouse(ctx context.Context, db DBTX, regID int64) (int64, *int64, error) {
	var org int64
	var wh *int64
	err := db.QueryRow(ctx, `SELECT organization_id, warehouse_id FROM cash_register WHERE id=$1`, regID).Scan(&org, &wh)
	return org, wh, err
}

// ShiftRepo — кассовые смены.
type ShiftRepo struct{}

func (ShiftRepo) Open(ctx context.Context, db DBTX, regID, userID int64, startCash float64) (int64, int64, error) {
	var org, id, number int64
	if err := db.QueryRow(ctx, `SELECT organization_id FROM cash_register WHERE id=$1`, regID).Scan(&org); err != nil {
		return 0, 0, err
	}
	if err := db.QueryRow(ctx, `SELECT COALESCE(MAX(shift_number),0)+1 FROM cash_shift WHERE cash_register_id=$1`, regID).Scan(&number); err != nil {
		return 0, 0, err
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO cash_shift(organization_id, cash_register_id, shift_number, opened_by_id, start_cash)
		VALUES($1,$2,$3,$4,$5) RETURNING id`, org, regID, number, userID, startCash).Scan(&id); err != nil {
		return 0, 0, err
	}
	return id, number, nil
}

func (ShiftRepo) GetOpen(ctx context.Context, db DBTX, regID int64) (int64, int64, error) {
	var id, number int64
	err := db.QueryRow(ctx, `
		SELECT id, shift_number FROM cash_shift WHERE cash_register_id=$1 AND status='OPEN'`, regID).Scan(&id, &number)
	return id, number, err
}

func (ShiftRepo) Report(ctx context.Context, db DBTX, shiftID int64) (model.ShiftReport, error) {
	r := model.ShiftReport{ShiftID: shiftID}
	err := db.QueryRow(ctx, `
		SELECT s.shift_number, COALESCE(s.start_cash,0),
			COUNT(*) FILTER (WHERE r.receipt_type='SALE'),
			COUNT(*) FILTER (WHERE r.receipt_type='RETURN'),
			COALESCE(SUM(r.payment_cash) FILTER (WHERE r.receipt_type='SALE'),0),
			COALESCE(SUM(r.payment_card) FILTER (WHERE r.receipt_type='SALE'),0),
			COALESCE(SUM(r.payment_cash) FILTER (WHERE r.receipt_type='RETURN'),0),
			COALESCE(SUM(r.payment_card) FILTER (WHERE r.receipt_type='RETURN'),0)
		FROM cash_shift s LEFT JOIN sales_receipt r ON r.shift_id = s.id
		WHERE s.id=$1 GROUP BY s.shift_number, s.start_cash`, shiftID).
		Scan(&r.ShiftNumber, &r.StartCash, &r.SaleCount, &r.ReturnCount,
			&r.CashSales, &r.CardSales, &r.CashReturns, &r.CardReturns)
	if err != nil {
		return r, err
	}
	r.ExpectedCash = model.Round2(r.StartCash + r.CashSales - r.CashReturns)
	return r, nil
}

func (ShiftRepo) Close(ctx context.Context, db DBTX, shiftID, userID int64, actual float64, z map[string]interface{}) error {
	res, err := db.Exec(ctx, `
		UPDATE cash_shift SET status='CLOSED', closed_at=NOW(), closed_by_id=$2,
			actual_cash=$3, x_report=$4, z_report=$4
		WHERE id=$1 AND status='OPEN'`, shiftID, userID, actual, z)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("already closed")
	}
	return nil
}

// ReceiptRepo — чеки.
type ReceiptRepo struct{}

// Insert создает чек с позициями, ставит в очередь ОФД, списывает коды и линкует брони.
// Итоги уже посчитаны вызывающим (model.LineTotals).
func (ReceiptRepo) Insert(ctx context.Context, db DBTX, org, reg, shift, cashierID int64,
	rtype string, lines []model.ReceiptLine, total, vatSum float64,
	payType string, payCash, payCard float64, baseID *int64, corrReason string) (int64, string, error) {
	change := model.Round2(payCash + payCard - total)
	var lastNum int64
	_ = db.QueryRow(ctx, `SELECT COALESCE(MAX(receipt_number::bigint),0) FROM sales_receipt WHERE cash_register_id=$1`, reg).Scan(&lastNum)
	number := strconv.FormatInt(lastNum+1, 10)
	marked := false
	for _, l := range lines {
		if l.Marked {
			marked = true
		}
	}
	var rid int64
	if err := db.QueryRow(ctx, `
		INSERT INTO sales_receipt(organization_id, shift_id, cash_register_id, cashier_id, receipt_number,
			receipt_type, base_receipt_id, correction_reason,
			total_amount, total_vat, payment_type, payment_cash, payment_card, change_amount, has_marked_goods)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		org, shift, reg, cashierID, number, rtype, baseID, corrReason,
		total, vatSum, payType, payCash, payCard, change, marked).Scan(&rid); err != nil {
		return 0, "", err
	}
	var allCodes []int64
	for _, l := range lines {
		t := model.Round2(l.Price*l.Qty - l.Discount)
		var itemID int64
		if err := db.QueryRow(ctx, `
			INSERT INTO sales_receipt_item(receipt_id, product_id, product_name, product_sku,
				quantity, price, vat_rate, vat_amount, total_amount, discount, is_marked,
				ffd_item_attribute, ffd_payment_method)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
			rid, l.ProductID, l.Name, l.SKU, l.Qty, l.Price, l.VAT,
			model.Round2(t*l.VAT/100), t, l.Discount, l.Marked, l.Attr, l.Method).Scan(&itemID); err != nil {
			return 0, "", err
		}
		if l.BookingID != nil {
			if _, err := db.Exec(ctx, `
				UPDATE service_booking SET sales_receipt_item_id=$2 WHERE id=$1`, *l.BookingID, itemID); err != nil {
				return 0, "", err
			}
		}
		allCodes = append(allCodes, l.CodeIDs...)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO ofd_send_status(receipt_id, organization_id) VALUES($1,$2)`, rid, org); err != nil {
		return 0, "", err
	}
	if len(allCodes) > 0 {
		if err := (MarkingRepo{}).Withdraw(ctx, db, org, rid, cashierID, allCodes); err != nil {
			return 0, "", err
		}
	}
	return rid, number, nil
}

func (ReceiptRepo) List(ctx context.Context, db DBTX, shiftID int64, limit int) []model.Receipt {
	q := `SELECT r.id, r.receipt_number, r.receipt_type, r.total_amount, r.payment_type,
		r.created_at::text, COALESCE(o.status,'?'), COALESCE(o.fiscal_sign,'')
		FROM sales_receipt r LEFT JOIN ofd_send_status o ON o.receipt_id=r.id`
	var args []interface{}
	if shiftID != 0 {
		q += ` WHERE r.shift_id=$1`
		args = append(args, shiftID)
	}
	q += ` ORDER BY r.id DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Receipt
	for rows.Next() {
		var r model.Receipt
		_ = rows.Scan(&r.ID, &r.ReceiptNumber, &r.ReceiptType, &r.TotalAmount, &r.PaymentType, &r.CreatedAt, &r.OFDStatus, &r.FiscalSign)
		out = append(out, r)
	}
	if out == nil {
		out = []model.Receipt{}
	}
	return out
}

// BaseForReturn возвращает связки исходного чека для возврата.
func (ReceiptRepo) BaseForReturn(ctx context.Context, db DBTX, baseID int64) (org, reg, shift int64, rtype string, err error) {
	err = db.QueryRow(ctx, `
		SELECT organization_id, cash_register_id, shift_id, receipt_type FROM sales_receipt WHERE id=$1`, baseID).
		Scan(&org, &reg, &shift, &rtype)
	return org, reg, shift, rtype, err
}

// BaseItem — позиция исходного чека для контроля возврата.
type BaseItem struct {
	Name   string
	SKU    string
	Price  float64
	VAT    float64
	Marked bool
	Sold   float64
}

func (ReceiptRepo) BaseItem(ctx context.Context, db DBTX, baseID, productID int64) (BaseItem, error) {
	var b BaseItem
	err := db.QueryRow(ctx, `
		SELECT product_name, product_sku, price, vat_rate, is_marked, quantity
		FROM sales_receipt_item WHERE receipt_id=$1 AND product_id=$2`, baseID, productID).
		Scan(&b.Name, &b.SKU, &b.Price, &b.VAT, &b.Marked, &b.Sold)
	return b, err
}

func (ReceiptRepo) ReturnedQty(ctx context.Context, db DBTX, baseID, productID int64) float64 {
	var q float64
	_ = db.QueryRow(ctx, `
		SELECT COALESCE(SUM(i.quantity),0) FROM sales_receipt r
		JOIN sales_receipt_item i ON i.receipt_id=r.id
		WHERE r.base_receipt_id=$1 AND i.product_id=$2 AND r.receipt_type='RETURN'`, baseID, productID).Scan(&q)
	return q
}

// SoldCodeInReceipt проверяет, что код продан в указанном чеке. Возвращает id кода.
func (ReceiptRepo) SoldCodeInReceipt(ctx context.Context, db DBTX, code string, baseID int64) (int64, error) {
	var cid int64
	var st string
	var linkRID *int64
	if err := db.QueryRow(ctx, `
		SELECT m.id, m.status, l.receipt_id
		FROM marking_code_pool m
		LEFT JOIN receipt_marking_link l ON l.marking_code_id=m.id
		WHERE m.code=$1 FOR UPDATE OF m`, code).Scan(&cid, &st, &linkRID); err != nil {
		return 0, fmt.Errorf("unknown marking code: %s", code)
	}
	if linkRID == nil || *linkRID != baseID || st != "SOLD" {
		return 0, fmt.Errorf("code not sold in base receipt: %s", code)
	}
	return cid, nil
}

// CheckBookingLink проверяет бронь для привязки к позиции чека.
func (ReceiptRepo) CheckBookingLink(ctx context.Context, db DBTX, bookingID, productID, orgID int64) error {
	var borg int64
	var bst string
	var bcnt int
	if err := db.QueryRow(ctx, `
		SELECT b.organization_id, b.status_code,
		       (SELECT COUNT(*) FROM service_booking_item bi
		        WHERE bi.booking_id=b.id AND bi.product_id=$2)
		FROM service_booking b WHERE b.id=$1`, bookingID, productID).
		Scan(&borg, &bst, &bcnt); err != nil || borg != orgID || bcnt == 0 {
		return fmt.Errorf("bad booking link")
	}
	if bst == "CANCELED" || bst == "NO_SHOW" || bst == "COMPLETED" {
		return fmt.Errorf("booking is closed")
	}
	return nil
}

// ShiftOpen проверяет, что смена открыта.
func (ReceiptRepo) ShiftOpen(ctx context.Context, db DBTX, shiftID int64) error {
	var st string
	if err := db.QueryRow(ctx, `SELECT status FROM cash_shift WHERE id=$1`, shiftID).Scan(&st); err != nil || st != "OPEN" {
		return fmt.Errorf("base shift closed")
	}
	return nil
}

// OfdRepo — очередь ОФД (используется воркером).
type OfdRepo struct{}

// OfdJob — задание воркера.
type OfdJob struct {
	ID         int64
	ReceiptID  int64
	OrgID      int64
	Attempt    int
	MaxRetries int
	FailFirst  int
	AutoSend   bool
}

func (OfdRepo) Poll(ctx context.Context, db DBTX) []OfdJob {
	rows, err := db.Query(ctx, `
		SELECT o.id, o.receipt_id, o.organization_id, o.send_attempt,
		       COALESCE(st.max_retries, 3), COALESCE(st.fail_first_attempts, 0),
		       COALESCE(st.auto_send_enabled, TRUE)
		FROM ofd_send_status o
		LEFT JOIN ofd_settings st ON st.organization_id = o.organization_id AND st.is_active
		WHERE o.status IN ('PENDING','RETRY')
		ORDER BY o.id LIMIT 20`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []OfdJob
	for rows.Next() {
		var j OfdJob
		if err := rows.Scan(&j.ID, &j.ReceiptID, &j.OrgID, &j.Attempt, &j.MaxRetries, &j.FailFirst, &j.AutoSend); err != nil {
			continue
		}
		out = append(out, j)
	}
	return out
}

func (OfdRepo) FailMark(ctx context.Context, db DBTX, id int64, attempt, maxRet int) {
	_, _ = db.Exec(ctx, `
		UPDATE ofd_send_status SET send_attempt=$1, last_attempt_at=NOW(),
			status=CASE WHEN $1::int >= $2::int THEN 'FAILED' ELSE 'RETRY' END,
			error_message='mock: OFD unavailable', updated_at=NOW() WHERE id=$3`,
		attempt, maxRet, id)
}

func (OfdRepo) Complete(ctx context.Context, db DBTX, id int64, attempt int, docNumber, sign, qr string) error {
	_, err := db.Exec(ctx, `
		UPDATE ofd_send_status SET send_attempt=$1, last_attempt_at=NOW(), status='COMPLETED',
			fiscal_document_number=$2, fiscal_sign=$3, qr_code_url=$4,
			error_message=NULL, updated_at=NOW() WHERE id=$5`,
		attempt, docNumber, sign, qr, id)
	return err
}

// OfdSettings — чтение/патч настроек (админка).
type OfdSettings struct {
	Provider   *string `json:"provider"`
	APIURL     *string `json:"api_url"`
	AutoSend   bool    `json:"auto_send_enabled"`
	MaxRetries int     `json:"max_retries"`
	FailFirst  int     `json:"fail_first_attempts"`
	IsActive   bool    `json:"is_active"`
}

func (OfdRepo) EnsureSettings(ctx context.Context, db DBTX, orgID int64) {
	_, _ = db.Exec(ctx, `
		INSERT INTO ofd_settings(organization_id) VALUES($1) ON CONFLICT (organization_id, provider) DO NOTHING`, orgID)
}

func (OfdRepo) GetSettings(ctx context.Context, db DBTX, orgID int64) (OfdSettings, error) {
	var s OfdSettings
	err := db.QueryRow(ctx, `
		SELECT provider, api_url, auto_send_enabled, max_retries, fail_first_attempts, is_active
		FROM ofd_settings WHERE organization_id=$1 AND is_active LIMIT 1`, orgID).
		Scan(&s.Provider, &s.APIURL, &s.AutoSend, &s.MaxRetries, &s.FailFirst, &s.IsActive)
	return s, err
}

func (OfdRepo) PatchSettings(ctx context.Context, db DBTX, orgID int64, failFirst *int, auto *bool) {
	if failFirst != nil {
		_, _ = db.Exec(ctx, `UPDATE ofd_settings SET fail_first_attempts=$1 WHERE organization_id=$2`, *failFirst, orgID)
	}
	if auto != nil {
		_, _ = db.Exec(ctx, `UPDATE ofd_settings SET auto_send_enabled=$1 WHERE organization_id=$2`, *auto, orgID)
	}
}
