package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/store"
)

type Marking struct {
	Store *store.Store
}

// EnsureGismtForOrg — дефолтные настройки ГИС МТ (mock) для организации.
func EnsureGismtForOrg(s *store.Store, c echo.Context, orgID int64) {
	_, _ = s.PG.Exec(c.Request().Context(), `
		INSERT INTO gismt_settings(organization_id) VALUES($1)
		ON CONFLICT (organization_id, provider) DO NOTHING`, orgID)
}

// EnsureGismtForOrgTx — то же внутри транзакции.
func EnsureGismtForOrgTx(tx pgx.Tx, c echo.Context, orgID int64) {
	_, _ = tx.Exec(c.Request().Context(), `
		INSERT INTO gismt_settings(organization_id) VALUES($1)
		ON CONFLICT (organization_id, provider) DO NOTHING`, orgID)
}

type registerCodesReq struct {
	OrgID     int64    `json:"org_id"`
	ProductID int64    `json:"product_id"`
	Codes     []string `json:"codes"`
	Batch     string   `json:"batch_number"`
}

// Register принимает коды в пул (AVAILABLE) + ставит RECEIVE в очередь ГИС МТ.
// Опционально batch_number — тогда создается пачка одним батчем.
func (h *Marking) Register(c echo.Context) error {
	var b registerCodesReq
	if err := c.Bind(&b); err != nil || b.OrgID == 0 || b.ProductID == 0 || len(b.Codes) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id/product_id/codes[] required"})
	}
	var gtin, pname string
	var marked bool
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT gtin, name, is_marked FROM catalog_product WHERE id=$1 AND is_active`, b.ProductID).
		Scan(&gtin, &pname, &marked)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no product"})
	}
	if !marked {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "product is not marked"})
	}
	type rej struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	var rejected []rej
	var clean []string
	seen := map[string]bool{}
	for _, raw := range b.Codes {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		if seen[code] {
			rejected = append(rejected, rej{code, "duplicate in request"})
			continue
		}
		seen[code] = true
		if gtin != "" && !strings.Contains(code, gtin) {
			rejected = append(rejected, rej{code, "gtin mismatch"})
			continue
		}
		clean = append(clean, code)
	}
	registered := 0
	dups := 0
	err = h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var batchID *int64
		if b.Batch != "" {
			var bid int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO marking_batch(organization_id, product_id, batch_number, total_codes)
				VALUES($1,$2,$3,$4) RETURNING id`, b.OrgID, b.ProductID, b.Batch, len(clean)).Scan(&bid); err != nil {
				return err
			}
			batchID = &bid
		}
		for _, code := range clean {
			var cid int64
			err := tx.QueryRow(ctx, `
				INSERT INTO marking_code_pool(organization_id, product_id, code, gtin, batch_id)
				VALUES($1,$2,$3,$4,$5) ON CONFLICT (code) DO NOTHING RETURNING id`,
				b.OrgID, b.ProductID, code, gtin, batchID).Scan(&cid)
			if err != nil {
				dups++
				continue
			}
			registered++
			_, _ = tx.Exec(ctx, `
				INSERT INTO gismt_queue(organization_id, marking_code_id, operation)
				VALUES($1,$2,'RECEIVE')`, b.OrgID, cid)
		}
		return nil
	})
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "register failed (duplicate batch?)"})
	}
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "marking.register",
		"Регистрация кодов маркировки", "product", &b.ProductID,
		map[string]interface{}{"registered": registered}, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"registered": registered, "duplicates": dups, "rejected": rejected, "product": pname,
	})
}

func (h *Marking) List(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	prodID := c.QueryParam("product_id")
	status := c.QueryParam("status")
	q := strings.TrimSpace(c.QueryParam("q"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := "WHERE m.organization_id=$1"
	args := []interface{}{orgID}
	if prodID != "" {
		args = append(args, prodID)
		where += ` AND m.product_id=$` + strconv.Itoa(len(args))
	}
	if status != "" {
		args = append(args, status)
		where += ` AND m.status=$` + strconv.Itoa(len(args))
	}
	if q != "" {
		args = append(args, "%"+q+"%")
		where += ` AND m.code ILIKE $` + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT m.id, m.code, m.status, m.product_id, p.name, m.batch_id,
		       m.sales_receipt_id, m.created_at::text
		FROM marking_code_pool m JOIN catalog_product p ON p.id=m.product_id
		`+where+` ORDER BY m.id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id, pid int64
		var code, st, pname, ts string
		var batch, receipt *int64
		_ = rows.Scan(&id, &code, &st, &pid, &pname, &batch, &receipt, &ts)
		out = append(out, map[string]interface{}{"id": id, "code": code, "status": st,
			"product_id": pid, "product_name": pname, "batch_id": batch,
			"sales_receipt_id": receipt, "created_at": ts})
	}
	return c.JSON(http.StatusOK, out)
}

// Check — локальная проверка кода: можно ли продать.
func (h *Marking) Check(c echo.Context) error {
	code := strings.TrimSpace(c.Param("code"))
	var id, pid int64
	var st, pname string
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT m.id, m.status, m.product_id, p.name
		FROM marking_code_pool m JOIN catalog_product p ON p.id=m.product_id
		WHERE m.code=$1`, code).Scan(&id, &st, &pid, &pname)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "code unknown"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": id, "code": code,
		"status": st, "product_id": pid, "product_name": pname, "can_sell": st == "AVAILABLE"})
}

type writeOffReq struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

func (h *Marking) WriteOff(c echo.Context) error {
	var b writeOffReq
	if err := c.Bind(&b); err != nil || b.Code == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "code required"})
	}
	x := middleware.CtxOf(c)
	var cid, org int64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var st string
		if err := tx.QueryRow(ctx, `
			SELECT id, organization_id, status FROM marking_code_pool WHERE code=$1 FOR UPDATE`, b.Code).
			Scan(&cid, &org, &st); err != nil {
			return err
		}
		if st != "AVAILABLE" {
			return echo.NewHTTPError(http.StatusConflict, "code not available: "+st)
		}
		if _, err := tx.Exec(ctx, `UPDATE marking_code_pool SET status='WRITTEN_OFF' WHERE id=$1`, cid); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `
			INSERT INTO gismt_queue(organization_id, marking_code_id, operation) VALUES($1,$2,'WRITE_OFF')`, org, cid)
		_, _ = tx.Exec(ctx, `
			INSERT INTO integration_log(organization_id, integration_type, direction, endpoint, request_data)
			VALUES($1,'GIS_MT','OUT','mock://gismt/write-off',$2)`, org,
			map[string]string{"code": b.Code, "reason": b.Reason, "by": x.Username})
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusNotFound, map[string]string{"error": "code unknown"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "written_off"})
}

func (h *Marking) Queue(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	status := c.QueryParam("status")
	q := `SELECT q.id, q.operation, q.status, q.send_attempt, m.code, q.receipt_id, q.error_message
		FROM gismt_queue q JOIN marking_code_pool m ON m.id=q.marking_code_id
		WHERE q.organization_id=$1`
	args := []interface{}{orgID}
	if status != "" {
		args = append(args, status)
		q += ` AND q.status=$2`
	}
	q += ` ORDER BY q.id DESC LIMIT 100`
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var op, st, code string
		var attempt int
		var receipt *int64
		var emsg *string
		_ = rows.Scan(&id, &op, &st, &attempt, &code, &receipt, &emsg)
		out = append(out, map[string]interface{}{"id": id, "operation": op, "status": st,
			"attempts": attempt, "code": code, "receipt_id": receipt, "error": emsg})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Marking) Log(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	itype := c.QueryParam("type")
	q := `SELECT id, integration_type, direction, endpoint, is_error, external_id, document_id, created_at::text
		FROM integration_log WHERE organization_id=$1`
	args := []interface{}{orgID}
	if itype != "" {
		args = append(args, itype)
		q += ` AND integration_type=$2`
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var it, dir, ts string
		var ep *string
		var isErr bool
		var ext *string
		var doc *int64
		_ = rows.Scan(&id, &it, &dir, &ep, &isErr, &ext, &doc, &ts)
		out = append(out, map[string]interface{}{"id": id, "type": it, "direction": dir,
			"endpoint": ep, "is_error": isErr, "external_id": ext, "document_id": doc, "at": ts})
	}
	return c.JSON(http.StatusOK, out)
}

// withdrawLocked списывает коды при продаже: проверка + SOLD + линк + очередь + used_codes.
// Вызывать внутри транзакции чека; коды уже должны быть собраны lockCodes.
func withdrawLocked(tx pgx.Tx, ctx context.Context, org, receiptID, cashierID int64, codeIDs []int64) error {
	for _, cid := range codeIDs {
		if _, err := tx.Exec(ctx, `
			UPDATE marking_code_pool SET status='SOLD', sold_at=NOW(), sales_receipt_id=$2 WHERE id=$1`, cid, receiptID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO receipt_marking_link(receipt_id, marking_code_id, cashier_id) VALUES($1,$2,$3)`,
			receiptID, cid, cashierID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO gismt_queue(organization_id, marking_code_id, operation, receipt_id)
			VALUES($1,$2,'WITHDRAW',$3)`, org, cid, receiptID); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `
			UPDATE marking_batch SET used_codes = used_codes + 1
			WHERE id = (SELECT batch_id FROM marking_code_pool WHERE id=$1) AND (SELECT batch_id FROM marking_code_pool WHERE id=$1) IS NOT NULL`, cid)
	}
	return nil
}

// returnLocked возвращает коды в оборот при возврате чека.
func returnLocked(tx pgx.Tx, ctx context.Context, org, baseReceiptID, returnReceiptID int64, codeIDs []int64) error {
	for _, cid := range codeIDs {
		if _, err := tx.Exec(ctx, `
			UPDATE marking_code_pool SET status='AVAILABLE', sold_at=NULL, sales_receipt_id=NULL WHERE id=$1`, cid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO gismt_queue(organization_id, marking_code_id, operation, receipt_id)
			VALUES($1,$2,'RETURN',$3)`, org, cid, returnReceiptID); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `
			UPDATE marking_batch SET used_codes = used_codes - 1
			WHERE id = (SELECT batch_id FROM marking_code_pool WHERE id=$1) AND (SELECT batch_id FROM marking_code_pool WHERE id=$1) IS NOT NULL`, cid)
		_ = baseReceiptID
	}
	return nil
}
