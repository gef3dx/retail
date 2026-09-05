package repository

import (
	"context"
	"fmt"
	"strings"

	"retail-backend/internal/model"
)

// MarkingRepo — коды маркировки: проверка, списание, возврат.
type MarkingRepo struct{}

// LockCodes проверяет коды (AVAILABLE, тот же товар и организация) и лочит их.
// Возвращает id кодов в порядке входных кодов.
func (MarkingRepo) LockCodes(ctx context.Context, db DBTX, org, productID int64, codes []string) ([]int64, error) {
	var ids []int64
	seen := map[string]bool{}
	for _, raw := range codes {
		code := strings.TrimSpace(raw)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		var id int64
		var st string
		var pid, porg int64
		if err := db.QueryRow(ctx, `
			SELECT id, status, product_id, organization_id FROM marking_code_pool WHERE code=$1 FOR UPDATE`, code).
			Scan(&id, &st, &pid, &porg); err != nil {
			return nil, fmt.Errorf("unknown marking code: %s", code)
		}
		if pid != productID {
			return nil, fmt.Errorf("code product mismatch: %s", code)
		}
		if porg != org {
			return nil, fmt.Errorf("code org mismatch: %s", code)
		}
		if st != "AVAILABLE" {
			return nil, fmt.Errorf("code not available (%s): %s", st, code)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Withdraw списывает коды при продаже: SOLD + линк к чеку + очередь WITHDRAW + used_codes.
func (MarkingRepo) Withdraw(ctx context.Context, db DBTX, org, receiptID, cashierID int64, codeIDs []int64) error {
	for _, cid := range codeIDs {
		if _, err := db.Exec(ctx, `
			UPDATE marking_code_pool SET status='SOLD', sold_at=NOW(), sales_receipt_id=$2 WHERE id=$1`, cid, receiptID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO receipt_marking_link(receipt_id, marking_code_id, cashier_id) VALUES($1,$2,$3)`,
			receiptID, cid, cashierID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO gismt_queue(organization_id, marking_code_id, operation, receipt_id)
			VALUES($1,$2,'WITHDRAW',$3)`, org, cid, receiptID); err != nil {
			return err
		}
		_, _ = db.Exec(ctx, `
			UPDATE marking_batch SET used_codes = used_codes + 1
			WHERE id = (SELECT batch_id FROM marking_code_pool WHERE id=$1) AND (SELECT batch_id FROM marking_code_pool WHERE id=$1) IS NOT NULL`, cid)
	}
	return nil
}

// Return возвращает коды в оборот при возврате чека.
func (MarkingRepo) Return(ctx context.Context, db DBTX, org, returnReceiptID int64, codeIDs []int64) error {
	for _, cid := range codeIDs {
		if _, err := db.Exec(ctx, `
			UPDATE marking_code_pool SET status='AVAILABLE', sold_at=NULL, sales_receipt_id=NULL WHERE id=$1`, cid); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO gismt_queue(organization_id, marking_code_id, operation, receipt_id)
			VALUES($1,$2,'RETURN',$3)`, org, cid, returnReceiptID); err != nil {
			return err
		}
		_, _ = db.Exec(ctx, `
			UPDATE marking_batch SET used_codes = used_codes - 1
			WHERE id = (SELECT batch_id FROM marking_code_pool WHERE id=$1) AND (SELECT batch_id FROM marking_code_pool WHERE id=$1) IS NOT NULL`, cid)
	}
	return nil
}

// CodeFilter — фильтры списка кодов.
type CodeFilter struct {
	OrgID     int64
	ProductID string
	Status    string
	Q         string
	Limit     int
}

// CodesRepo — пул кодов, очередь ГИС МТ, лог интеграций (чтение).
type CodesRepo struct{}

func (CodesRepo) ProductForRegister(ctx context.Context, db DBTX, productID int64) (gtin, name string, marked bool, err error) {
	err = db.QueryRow(ctx, `
		SELECT gtin, name, is_marked FROM catalog_product WHERE id=$1 AND is_active`, productID).
		Scan(&gtin, &name, &marked)
	return gtin, name, marked, err
}

func (CodesRepo) CreateBatch(ctx context.Context, db DBTX, orgID, productID int64, batch string, total int) (int64, error) {
	var bid int64
	err := db.QueryRow(ctx, `
		INSERT INTO marking_batch(organization_id, product_id, batch_number, total_codes)
		VALUES($1,$2,$3,$4) RETURNING id`, orgID, productID, batch, total).Scan(&bid)
	return bid, err
}

// InsertCode вставляет код (ON CONFLICT DO NOTHING) + ставит RECEIVE.
// Возвращает (id, inserted).
func (CodesRepo) InsertCode(ctx context.Context, db DBTX, orgID, productID int64, code, gtin string, batchID *int64) (int64, bool) {
	var cid int64
	err := db.QueryRow(ctx, `
		INSERT INTO marking_code_pool(organization_id, product_id, code, gtin, batch_id)
		VALUES($1,$2,$3,$4,$5) ON CONFLICT (code) DO NOTHING RETURNING id`,
		orgID, productID, code, gtin, batchID).Scan(&cid)
	if err != nil {
		return 0, false
	}
	_, _ = db.Exec(ctx, `
		INSERT INTO gismt_queue(organization_id, marking_code_id, operation)
		VALUES($1,$2,'RECEIVE')`, orgID, cid)
	return cid, true
}

func (CodesRepo) List(ctx context.Context, db DBTX, f CodeFilter) []model.MarkingCode {
	where := "WHERE m.organization_id=$1"
	args := []interface{}{f.OrgID}
	if f.ProductID != "" {
		args = append(args, f.ProductID)
		where += ` AND m.product_id=$` + itoa(len(args))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where += ` AND m.status=$` + itoa(len(args))
	}
	if f.Q != "" {
		args = append(args, "%"+f.Q+"%")
		where += ` AND m.code ILIKE $` + itoa(len(args))
	}
	args = append(args, f.Limit)
	rows, err := db.Query(ctx, `
		SELECT m.id, m.code, m.status, m.product_id, p.name, m.batch_id,
		       m.sales_receipt_id, m.created_at::text
		FROM marking_code_pool m JOIN catalog_product p ON p.id=m.product_id
		`+where+` ORDER BY m.id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.MarkingCode
	for rows.Next() {
		var c model.MarkingCode
		_ = rows.Scan(&c.ID, &c.Code, &c.Status, &c.ProductID, &c.ProductName, &c.BatchID, &c.SalesReceiptID, &c.CreatedAt)
		out = append(out, c)
	}
	if out == nil {
		out = []model.MarkingCode{}
	}
	return out
}

func (CodesRepo) Check(ctx context.Context, db DBTX, code string) (model.MarkingCheck, error) {
	var c model.MarkingCheck
	c.Code = code
	err := db.QueryRow(ctx, `
		SELECT m.id, m.status, m.product_id, p.name
		FROM marking_code_pool m JOIN catalog_product p ON p.id=m.product_id
		WHERE m.code=$1`, code).Scan(&c.ID, &c.Status, &c.ProductID, &c.ProductName)
	if err != nil {
		return c, err
	}
	c.CanSell = c.Status == "AVAILABLE"
	return c, nil
}

// WriteOff списывает код (только AVAILABLE) + очередь + лог.
func (CodesRepo) WriteOff(ctx context.Context, db DBTX, code, reason, by string) error {
	var cid, org int64
	var st string
	if err := db.QueryRow(ctx, `
		SELECT id, organization_id, status FROM marking_code_pool WHERE code=$1 FOR UPDATE`, code).
		Scan(&cid, &org, &st); err != nil {
		return errNotFound
	}
	if st != "AVAILABLE" {
		return errConflict("code not available: " + st)
	}
	if _, err := db.Exec(ctx, `UPDATE marking_code_pool SET status='WRITTEN_OFF' WHERE id=$1`, cid); err != nil {
		return err
	}
	_, _ = db.Exec(ctx, `
		INSERT INTO gismt_queue(organization_id, marking_code_id, operation) VALUES($1,$2,'WRITE_OFF')`, org, cid)
	_, _ = db.Exec(ctx, `
		INSERT INTO integration_log(organization_id, integration_type, direction, endpoint, request_data)
		VALUES($1,'GIS_MT','OUT','mock://gismt/write-off',$2)`, org,
		map[string]string{"code": code, "reason": reason, "by": by})
	return nil
}

func (CodesRepo) Queue(ctx context.Context, db DBTX, orgID int64, status string) []model.GismtQueueItem {
	q := `SELECT q.id, q.operation, q.status, q.send_attempt, m.code, q.receipt_id, q.error_message
		FROM gismt_queue q JOIN marking_code_pool m ON m.id=q.marking_code_id
		WHERE q.organization_id=$1`
	args := []interface{}{orgID}
	if status != "" {
		args = append(args, status)
		q += ` AND q.status=$2`
	}
	q += ` ORDER BY q.id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.GismtQueueItem
	for rows.Next() {
		var i model.GismtQueueItem
		_ = rows.Scan(&i.ID, &i.Operation, &i.Status, &i.Attempts, &i.Code, &i.ReceiptID, &i.Error)
		out = append(out, i)
	}
	if out == nil {
		out = []model.GismtQueueItem{}
	}
	return out
}

func (CodesRepo) Log(ctx context.Context, db DBTX, orgID int64, itype string) []model.IntegrationLogEntry {
	q := `SELECT id, integration_type, direction, endpoint, is_error, external_id, document_id, created_at::text
		FROM integration_log WHERE organization_id=$1`
	args := []interface{}{orgID}
	if itype != "" {
		args = append(args, itype)
		q += ` AND integration_type=$2`
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.IntegrationLogEntry
	for rows.Next() {
		var e model.IntegrationLogEntry
		_ = rows.Scan(&e.ID, &e.Type, &e.Direction, &e.Endpoint, &e.IsError, &e.ExternalID, &e.DocumentID, &e.At)
		out = append(out, e)
	}
	if out == nil {
		out = []model.IntegrationLogEntry{}
	}
	return out
}

// GismtJob — задание воркера ГИС МТ.
type GismtJob struct {
	ID                         int64
	OrgID                      int64
	CodeID                     int64
	Op                         string
	ReceiptID                  *int64
	Attempt, MaxRet, FailFirst int
	AutoSend                   bool
	Code                       string
}

// GismtRepo — очередь ГИС МТ для воркера.
type GismtRepo struct{}

func (GismtRepo) Poll(ctx context.Context, db DBTX) []GismtJob {
	rows, err := db.Query(ctx, `
		SELECT q.id, q.organization_id, q.marking_code_id, q.operation, q.receipt_id,
		       q.send_attempt, COALESCE(st.max_retries, 5), COALESCE(st.fail_first_attempts, 0),
		       COALESCE(st.auto_send_enabled, TRUE), m.code
		FROM gismt_queue q
		JOIN marking_code_pool m ON m.id = q.marking_code_id
		LEFT JOIN gismt_settings st ON st.organization_id = q.organization_id AND st.is_active
		WHERE q.status IN ('PENDING','RETRY')
		ORDER BY q.id LIMIT 20`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []GismtJob
	for rows.Next() {
		var j GismtJob
		if err := rows.Scan(&j.ID, &j.OrgID, &j.CodeID, &j.Op, &j.ReceiptID,
			&j.Attempt, &j.MaxRet, &j.FailFirst, &j.AutoSend, &j.Code); err != nil {
			continue
		}
		out = append(out, j)
	}
	return out
}

func (GismtRepo) FailMark(ctx context.Context, db DBTX, id int64, attempt, maxRet int) {
	_, _ = db.Exec(ctx, `
		UPDATE gismt_queue SET send_attempt=$1, last_attempt_at=NOW(),
			status=CASE WHEN $1::int >= $2::int THEN 'FAILED' ELSE 'RETRY' END,
			error_message='mock: GIS MT unavailable', updated_at=NOW() WHERE id=$3`,
		attempt, maxRet, id)
}

func (GismtRepo) Complete(ctx context.Context, db DBTX, id int64, attempt int) error {
	_, err := db.Exec(ctx, `
		UPDATE gismt_queue SET send_attempt=$1, last_attempt_at=NOW(), status='COMPLETED',
			error_message=NULL, updated_at=NOW() WHERE id=$2`, attempt, id)
	return err
}

func (GismtRepo) LogIntegration(ctx context.Context, db DBTX, orgID int64, endpoint string,
	req, resp []byte, extID string, receiptID *int64) {
	_, _ = db.Exec(ctx, `
		INSERT INTO integration_log(organization_id, integration_type, direction, endpoint,
			request_data, response_data, external_id, document_id)
		VALUES($1,'GIS_MT','OUT',$2,$3,$4,$5,$6)`,
		orgID, endpoint, req, resp, extID, receiptID)
}
