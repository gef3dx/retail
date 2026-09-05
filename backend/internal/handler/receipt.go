package handler

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"retail-backend/internal/middleware"
	"retail-backend/internal/notify"
	"retail-backend/internal/store"
)

type Receipt struct {
	Store *store.Store
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// ---------- Registers ----------

func (h *Receipt) ListRegisters(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	q := `SELECT id, organization_id, reg_number, model, status FROM cash_register`
	var args []interface{}
	if orgID != 0 {
		q += ` WHERE organization_id=$1`
		args = append(args, orgID)
	}
	q += ` ORDER BY id`
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id, org int64
		var reg, model, st string
		_ = rows.Scan(&id, &org, &reg, &model, &st)
		out = append(out, map[string]interface{}{"id": id, "organization_id": org, "reg_number": reg, "model": model, "status": st})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Receipt) CreateRegister(c echo.Context) error {
	var b struct {
		OrganizationID int64  `json:"organization_id"`
		RegNumber      string `json:"reg_number"`
		Model          string `json:"model"`
		Address        string `json:"address"`
	}
	if err := c.Bind(&b); err != nil || b.OrganizationID == 0 || b.RegNumber == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "organization_id/reg_number required"})
	}
	if b.Model == "" {
		b.Model = "MOCK-KKT"
	}
	var id int64
	if err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO cash_register(organization_id, reg_number, model, installation_address)
		VALUES($1,$2,$3,NULLIF($4,'')) RETURNING id`, b.OrganizationID, b.RegNumber, b.Model, b.Address).Scan(&id); err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "duplicate reg_number"})
	}
	// Дефолтные настройки ОФД (mock) для организации.
	_, _ = h.Store.PG.Exec(c.Request().Context(), `
		INSERT INTO ofd_settings(organization_id) VALUES($1) ON CONFLICT (organization_id, provider) DO NOTHING`, b.OrganizationID)
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

// PatchRegister привязывает склад (контроль остатков) и статус кассы.
func (h *Receipt) PatchRegister(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if v, ok := raw["warehouse_id"].(float64); ok {
		if v == 0 {
			_, _ = h.Store.PG.Exec(c.Request().Context(), `UPDATE cash_register SET warehouse_id=NULL WHERE id=$1`, id)
		} else {
			res, err := h.Store.PG.Exec(c.Request().Context(), `
				UPDATE cash_register r SET warehouse_id=$2 FROM warehouse w
				WHERE r.id=$1 AND w.id=$2 AND w.organization_id=r.organization_id`, id, int64(v))
			if err != nil || res.RowsAffected() == 0 {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad warehouse (other org?)"})
			}
		}
	}
	if v, ok := raw["status"].(string); ok && (v == "ACTIVE" || v == "INACTIVE" || v == "BLOCKED") {
		_, _ = h.Store.PG.Exec(c.Request().Context(), `UPDATE cash_register SET status=$2 WHERE id=$1`, id, v)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ---------- Shifts ----------

func (h *Receipt) OpenShift(c echo.Context) error {
	var b struct {
		CashRegisterID int64   `json:"cash_register_id"`
		StartCash      float64 `json:"start_cash"`
	}
	if err := c.Bind(&b); err != nil || b.CashRegisterID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cash_register_id required"})
	}
	x := middleware.CtxOf(c)
	var id, number int64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var org int64
		if err := tx.QueryRow(ctx, `SELECT organization_id FROM cash_register WHERE id=$1 FOR UPDATE`, b.CashRegisterID).Scan(&org); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(shift_number),0)+1 FROM cash_shift WHERE cash_register_id=$1`, b.CashRegisterID).Scan(&number); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO cash_shift(organization_id, cash_register_id, shift_number, opened_by_id, start_cash)
			VALUES($1,$2,$3,$4,$5) RETURNING id`, org, b.CashRegisterID, number, x.UserID, b.StartCash).Scan(&id); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "open failed (shift already open?)"})
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": id, "shift_number": number})
}

func (h *Receipt) OpenShiftForRegister(c echo.Context) error {
	regID, _ := strconv.ParseInt(c.QueryParam("register_id"), 10, 64)
	if regID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "register_id required"})
	}
	var id, number int64
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT id, shift_number FROM cash_shift WHERE cash_register_id=$1 AND status='OPEN'`, regID).Scan(&id, &number)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no open shift"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": id, "shift_number": number})
}

type shiftReport struct {
	ShiftID      int64   `json:"shift_id"`
	ShiftNumber  int64   `json:"shift_number"`
	SaleCount    int     `json:"sale_count"`
	ReturnCount  int     `json:"return_count"`
	CashSales    float64 `json:"cash_sales"`
	CardSales    float64 `json:"card_sales"`
	CashReturns  float64 `json:"cash_returns"`
	CardReturns  float64 `json:"card_returns"`
	StartCash    float64 `json:"start_cash"`
	ExpectedCash float64 `json:"expected_cash"`
}

func (h *Receipt) report(ctx echo.Context, shiftID int64) (*shiftReport, error) {
	r := &shiftReport{ShiftID: shiftID}
	err := h.Store.PG.QueryRow(ctx.Request().Context(), `
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
		return nil, err
	}
	r.ExpectedCash = round2(r.StartCash + r.CashSales - r.CashReturns)
	return r, nil
}

func (h *Receipt) XReport(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rep, err := h.report(c, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no shift"})
	}
	return c.JSON(http.StatusOK, rep)
}

func (h *Receipt) CloseShift(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var b struct {
		ActualCash *float64 `json:"actual_cash"`
	}
	_ = c.Bind(&b)
	x := middleware.CtxOf(c)
	rep, err := h.report(c, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no shift"})
	}
	actual := rep.ExpectedCash
	if b.ActualCash != nil {
		actual = *b.ActualCash
	}
	z := map[string]interface{}{
		"sale_count": rep.SaleCount, "return_count": rep.ReturnCount,
		"cash_sales": rep.CashSales, "card_sales": rep.CardSales,
		"cash_returns": rep.CashReturns, "card_returns": rep.CardReturns,
		"start_cash": rep.StartCash, "expected_cash": rep.ExpectedCash,
		"actual_cash": actual, "discrepancy": round2(actual - rep.ExpectedCash),
	}
	res, err := h.Store.PG.Exec(c.Request().Context(), `
		UPDATE cash_shift SET status='CLOSED', closed_at=NOW(), closed_by_id=$2,
			actual_cash=$3, x_report=$4, z_report=$4
		WHERE id=$1 AND status='OPEN'`, id, x.UserID, actual, z)
	if err != nil || res.RowsAffected() == 0 {
		return c.JSON(http.StatusConflict, map[string]string{"error": "close failed (already closed?)"})
	}
	return c.JSON(http.StatusOK, z)
}

// ---------- Sell / Return / Correction ----------

type sellItem struct {
	ProductID *int64   `json:"product_id"`
	Code      string   `json:"code"`
	Quantity  float64  `json:"quantity"`
	Price     *float64 `json:"price"`
	Discount  float64  `json:"discount"`
	ItemAttr  string   `json:"ffd_item_attribute"`
	PayMethod string   `json:"ffd_payment_method"`
	// Коды маркировки (обязательны для маркированного товара, по одному на единицу).
	MarkingCodes []string `json:"marking_codes"`
}

type sellReq struct {
	CashRegisterID int64      `json:"cash_register_id"`
	Items          []sellItem `json:"items"`
	PaymentType    string     `json:"payment_type"`
	PaymentCash    float64    `json:"payment_cash"`
	PaymentCard    float64    `json:"payment_card"`
}

type line struct {
	productID int64
	name      string
	sku       string
	qty       float64
	price     float64
	vat       float64
	discount  float64
	marked    bool
	attr      string
	method    string
	codeIDs   []int64
}

// lockCodes проверяет коды маркировки (AVAILABLE, тот же товар и организация) и лочит их.
func lockCodes(tx pgx.Tx, ctx context.Context, org, productID int64, codes []string) ([]int64, error) {
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
		if err := tx.QueryRow(ctx, `
			SELECT id, status, product_id, organization_id FROM marking_code_pool WHERE code=$1 FOR UPDATE`, code).
			Scan(&id, &st, &pid, &porg); err != nil {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "unknown marking code: "+code)
		}
		if pid != productID {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "code product mismatch: "+code)
		}
		if porg != org {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "code org mismatch: "+code)
		}
		if st != "AVAILABLE" {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "code not available ("+st+"): "+code)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// resolveItems проверяет товары, подставляет цену (явная → розница → базовая).
func (h *Receipt) resolveItems(c echo.Context, tx pgx.Tx, org int64, in []sellItem) ([]line, error) {
	var out []line
	for _, it := range in {
		if it.Quantity <= 0 {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "quantity must be > 0")
		}
		var id int64
		var name, sku string
		var vat float64
		var marked bool
		var active bool
		var err error
		if it.ProductID != nil {
			err = tx.QueryRow(c.Request().Context(), `
				SELECT id, name, sku, vat_rate, is_marked, is_active FROM catalog_product WHERE id=$1`, *it.ProductID).
				Scan(&id, &name, &sku, &vat, &marked, &active)
		} else if it.Code != "" {
			err = tx.QueryRow(c.Request().Context(), `
				SELECT p.id, p.name, p.sku, p.vat_rate, p.is_marked, p.is_active
				FROM catalog_product p LEFT JOIN product_packaging pk ON pk.product_id=p.id AND pk.gtin_packaging=$1
				WHERE p.sku=$1 OR p.gtin=$1 OR pk.gtin_packaging=$1 LIMIT 1`, it.Code).
				Scan(&id, &name, &sku, &vat, &marked, &active)
		} else {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "product_id or code required")
		}
		if err != nil {
			return nil, echo.NewHTTPError(http.StatusNotFound, "product not found")
		}
		if !active {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "product inactive")
		}
		price := 0.0
		hasPrice := false
		if it.Price != nil {
			price, hasPrice = *it.Price, true
		} else {
			var rp *float64
			_ = tx.QueryRow(c.Request().Context(), `
				SELECT pp.price FROM product_price pp JOIN price_type pt ON pt.id=pp.price_type_id
				WHERE pp.product_id=$1 AND pt.organization_id=$2 AND pt.price_kind='RETAIL'
				  AND pp.valid_from <= CURRENT_DATE AND (pp.valid_to IS NULL OR pp.valid_to >= CURRENT_DATE)
				ORDER BY pp.valid_from DESC LIMIT 1`, id, org).Scan(&rp)
			if rp != nil {
				price, hasPrice = *rp, true
			} else {
				var bp *float64
				_ = tx.QueryRow(c.Request().Context(), `SELECT base_price FROM catalog_product WHERE id=$1`, id).Scan(&bp)
				if bp != nil {
					price, hasPrice = *bp, true
				}
			}
		}
		if !hasPrice || price < 0 {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "price missing/invalid")
		}
		attr := it.ItemAttr
		if attr == "" {
			attr = "GOOD"
			if marked {
				attr = "MARKED"
			}
		}
		method := it.PayMethod
		if method == "" {
			method = "FULL"
		}
		var codeIDs []int64
		if marked {
			if len(it.MarkingCodes) != int(it.Quantity) {
				return nil, echo.NewHTTPError(http.StatusBadRequest, "marked product needs one code per unit")
			}
			var err error
			codeIDs, err = lockCodes(tx, c.Request().Context(), org, id, it.MarkingCodes)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, line{id, name, sku, it.Quantity, round2(price), vat, round2(it.Discount), marked, attr, method, codeIDs})
	}
	return out, nil
}

func totals(lines []line) (total, vat float64, marked bool) {
	for _, l := range lines {
		t := round2(l.price*l.qty - l.discount)
		total += t
		vat += round2(t * l.vat / 100)
		if l.marked {
			marked = true
		}
	}
	return round2(total), round2(vat), marked
}

// lineQty сворачивает позиции в карту product → qty для складских операций.
func lineQty(lines []line) map[int64]float64 {
	m := map[int64]float64{}
	for _, l := range lines {
		m[l.productID] += l.qty
	}
	return m
}

func (h *Receipt) insertReceipt(c echo.Context, tx pgx.Tx, org, reg, shift int64, rtype string,
	lines []line, payType string, payCash, payCard float64, baseID *int64, corrReason string) (int64, string, float64, error) {
	ctx := c.Request().Context()
	total, vatSum, marked := totals(lines)
	paid := round2(payCash + payCard)
	if paid < total {
		return 0, "", 0, echo.NewHTTPError(http.StatusBadRequest, "paid < total")
	}
	change := round2(paid - total)
	var lastNum int64
	_ = tx.QueryRow(ctx, `SELECT COALESCE(MAX(receipt_number::bigint),0) FROM sales_receipt WHERE cash_register_id=$1`, reg).Scan(&lastNum)
	number := strconv.FormatInt(lastNum+1, 10)
	x := middleware.CtxOf(c)
	var rid int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO sales_receipt(organization_id, shift_id, cash_register_id, cashier_id, receipt_number,
			receipt_type, base_receipt_id, correction_reason,
			total_amount, total_vat, payment_type, payment_cash, payment_card, change_amount, has_marked_goods)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		org, shift, reg, x.UserID, number, rtype, baseID, corrReason,
		total, vatSum, payType, payCash, payCard, change, marked).Scan(&rid); err != nil {
		return 0, "", 0, err
	}
	for _, l := range lines {
		t := round2(l.price*l.qty - l.discount)
		if _, err := tx.Exec(ctx, `
			INSERT INTO sales_receipt_item(receipt_id, product_id, product_name, product_sku,
				quantity, price, vat_rate, vat_amount, total_amount, discount, is_marked,
				ffd_item_attribute, ffd_payment_method)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			rid, l.productID, l.name, l.sku, l.qty, l.price, l.vat,
			round2(t*l.vat/100), t, l.discount, l.marked, l.attr, l.method); err != nil {
			return 0, "", 0, err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ofd_send_status(receipt_id, organization_id) VALUES($1,$2)`, rid, org); err != nil {
		return 0, "", 0, err
	}
	// Списание кодов маркировки (этап 5).
	var allCodes []int64
	for _, l := range lines {
		allCodes = append(allCodes, l.codeIDs...)
	}
	if len(allCodes) > 0 {
		if err := withdrawLocked(tx, ctx, org, rid, x.UserID, allCodes); err != nil {
			return 0, "", 0, err
		}
	}
	return rid, number, total, nil
}

func (h *Receipt) Sell(c echo.Context) error {
	var b sellReq
	if err := c.Bind(&b); err != nil || b.CashRegisterID == 0 || len(b.Items) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cash_register_id/items required"})
	}
	if b.PaymentType == "" {
		b.PaymentType = "CASH"
	}
	x := middleware.CtxOf(c)
	var rid int64
	var number string
	var saleTotal float64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var org int64
		var warehouse *int64
		if err := tx.QueryRow(ctx, `SELECT organization_id, warehouse_id FROM cash_register WHERE id=$1`, b.CashRegisterID).Scan(&org, &warehouse); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no register")
		}
		var shift int64
		if err := tx.QueryRow(ctx, `SELECT id FROM cash_shift WHERE cash_register_id=$1 AND status='OPEN'`, b.CashRegisterID).Scan(&shift); err != nil {
			return echo.NewHTTPError(http.StatusConflict, "shift not open")
		}
		lines, err := h.resolveItems(c, tx, org, b.Items)
		if err != nil {
			return err
		}
		if warehouse != nil {
			if err := deductLocked(tx, ctx, *warehouse, lineQty(lines)); err != nil {
				return err
			}
		}
		rid, number, saleTotal, err = h.insertReceipt(c, tx, org, b.CashRegisterID, shift, "SALE", lines, b.PaymentType, b.PaymentCash, b.PaymentCard, nil, "")
		if err != nil {
			return err
		}
		notify.EnqueueTx(tx, ctx, org, "RECEIPT_SOLD", []string{"WEB"},
			notify.RecipientOf(ctx, h.Store, x.UserID), "", "",
			map[string]interface{}{"receipt_number": number, "total_amount": saleTotal,
				"payment_type": b.PaymentType, "fiscal_doc": ""}, "receipt", &rid, 5)
		if warehouse != nil {
			notify.CheckLowStockTx(tx, ctx, h.Store, org, *warehouse)
		}
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "sell failed"})
	}
	h.Store.Audit(c.Request().Context(), &x.UserID, "receipt.sell", "Пробитие чека "+number, "receipt", &rid, b, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": rid, "receipt_number": number})
}

type returnReq struct {
	BaseReceiptID int64 `json:"base_receipt_id"`
	Items         []struct {
		ProductID    int64    `json:"product_id"`
		Quantity     float64  `json:"quantity"`
		MarkingCodes []string `json:"marking_codes"`
	} `json:"items"`
	PaymentType string  `json:"payment_type"`
	PaymentCash float64 `json:"payment_cash"`
	PaymentCard float64 `json:"payment_card"`
}

func (h *Receipt) Return(c echo.Context) error {
	var b returnReq
	if err := c.Bind(&b); err != nil || b.BaseReceiptID == 0 || len(b.Items) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "base_receipt_id/items required"})
	}
	if b.PaymentType == "" {
		b.PaymentType = "CASH"
	}
	var rid int64
	var number string
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var org, reg, shift int64
		var rtype string
		if err := tx.QueryRow(ctx, `
			SELECT organization_id, cash_register_id, shift_id, receipt_type FROM sales_receipt WHERE id=$1`, b.BaseReceiptID).
			Scan(&org, &reg, &shift, &rtype); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "base receipt not found")
		}
		if rtype != "SALE" {
			return echo.NewHTTPError(http.StatusBadRequest, "only SALE can be returned")
		}
		// Смена исходного чека должна быть открыта (возврат бьется в ту же смену).
		var st string
		if err := tx.QueryRow(ctx, `SELECT status FROM cash_shift WHERE id=$1`, shift).Scan(&st); err != nil || st != "OPEN" {
			return echo.NewHTTPError(http.StatusConflict, "base shift closed")
		}
		var lines []line
		var retCodes []int64
		for _, it := range b.Items {
			if it.Quantity <= 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "quantity must be > 0")
			}
			var sold, already float64
			var name, sku string
			var price, vat float64
			var marked bool
			if err := tx.QueryRow(ctx, `
				SELECT product_name, product_sku, price, vat_rate, is_marked, quantity
				FROM sales_receipt_item WHERE receipt_id=$1 AND product_id=$2`, b.BaseReceiptID, it.ProductID).
				Scan(&name, &sku, &price, &vat, &marked, &sold); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "item not in base receipt")
			}
			_ = tx.QueryRow(ctx, `
				SELECT COALESCE(SUM(i.quantity),0) FROM sales_receipt r
				JOIN sales_receipt_item i ON i.receipt_id=r.id
				WHERE r.base_receipt_id=$1 AND i.product_id=$2 AND r.receipt_type='RETURN'`, b.BaseReceiptID, it.ProductID).Scan(&already)
			if it.Quantity > sold-already {
				return echo.NewHTTPError(http.StatusBadRequest, "return qty exceeds sold")
			}
			if marked {
				if len(it.MarkingCodes) != int(it.Quantity) {
					return echo.NewHTTPError(http.StatusBadRequest, "marked return needs one code per unit")
				}
				// Коды должны быть из исходного чека и все еще SOLD.
				for _, raw := range it.MarkingCodes {
					code := strings.TrimSpace(raw)
					var cid int64
					var st string
					var linkRID *int64
					if err := tx.QueryRow(ctx, `
						SELECT m.id, m.status, l.receipt_id
						FROM marking_code_pool m
						LEFT JOIN receipt_marking_link l ON l.marking_code_id=m.id
						WHERE m.code=$1 FOR UPDATE OF m`, code).Scan(&cid, &st, &linkRID); err != nil {
						return echo.NewHTTPError(http.StatusBadRequest, "unknown marking code: "+code)
					}
					if linkRID == nil || *linkRID != b.BaseReceiptID || st != "SOLD" {
						return echo.NewHTTPError(http.StatusBadRequest, "code not sold in base receipt: "+code)
					}
					retCodes = append(retCodes, cid)
				}
			}
			lines = append(lines, line{it.ProductID, name, sku, it.Quantity, price, vat, 0, marked, "GOOD", "FULL", nil})
		}
		var err error
		rid, number, _, err = h.insertReceipt(c, tx, org, reg, shift, "RETURN", lines, b.PaymentType, b.PaymentCash, b.PaymentCard, &b.BaseReceiptID, "")
		if err != nil {
			return err
		}
		if len(retCodes) > 0 {
			if err := returnLocked(tx, ctx, org, b.BaseReceiptID, rid, retCodes); err != nil {
				return err
			}
		}
		// Возврат остатков на склад кассы (если привязан).
		var warehouse *int64
		_ = tx.QueryRow(ctx, `SELECT warehouse_id FROM cash_register WHERE id=$1`, reg).Scan(&warehouse)
		if warehouse != nil {
			if err := addLocked(tx, ctx, *warehouse, lineQty(lines)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "return failed"})
	}
	x := middleware.CtxOf(c)
	h.Store.Audit(c.Request().Context(), &x.UserID, "receipt.return", "Возврат по чеку", "receipt", &rid, b, clientIP(c), c.Request().UserAgent(), true, "")
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": rid, "receipt_number": number})
}

type corrReq struct {
	CashRegisterID int64      `json:"cash_register_id"`
	Items          []sellItem `json:"items"`
	PaymentType    string     `json:"payment_type"`
	PaymentCash    float64    `json:"payment_cash"`
	PaymentCard    float64    `json:"payment_card"`
	Reason         string     `json:"reason"`
}

func (h *Receipt) Correction(c echo.Context) error {
	var b corrReq
	if err := c.Bind(&b); err != nil || b.CashRegisterID == 0 || len(b.Items) == 0 || b.Reason == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cash_register_id/items/reason required"})
	}
	if b.PaymentType == "" {
		b.PaymentType = "CASH"
	}
	x := middleware.CtxOf(c)
	var rid int64
	var number string
	var saleTotal float64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var org int64
		var warehouse *int64
		if err := tx.QueryRow(ctx, `SELECT organization_id, warehouse_id FROM cash_register WHERE id=$1`, b.CashRegisterID).Scan(&org, &warehouse); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no register")
		}
		var shift int64
		if err := tx.QueryRow(ctx, `SELECT id FROM cash_shift WHERE cash_register_id=$1 AND status='OPEN'`, b.CashRegisterID).Scan(&shift); err != nil {
			return echo.NewHTTPError(http.StatusConflict, "shift not open")
		}
		lines, err := h.resolveItems(c, tx, org, b.Items)
		if err != nil {
			return err
		}
		if warehouse != nil {
			if err := deductLocked(tx, ctx, *warehouse, lineQty(lines)); err != nil {
				return err
			}
		}
		rid, number, saleTotal, err = h.insertReceipt(c, tx, org, b.CashRegisterID, shift, "CORRECTION", lines, b.PaymentType, b.PaymentCash, b.PaymentCard, nil, b.Reason)
		if err != nil {
			return err
		}
		notify.EnqueueTx(tx, ctx, org, "RECEIPT_SOLD", []string{"WEB"},
			notify.RecipientOf(ctx, h.Store, x.UserID), "", "",
			map[string]interface{}{"receipt_number": number, "total_amount": saleTotal,
				"payment_type": b.PaymentType, "fiscal_doc": ""}, "receipt", &rid, 5)
		if warehouse != nil {
			notify.CheckLowStockTx(tx, ctx, h.Store, org, *warehouse)
		}
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "correction failed"})
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"id": rid, "receipt_number": number})
}

// ---------- Lists + OFD settings ----------

func (h *Receipt) ListReceipts(c echo.Context) error {
	shiftID, _ := strconv.ParseInt(c.QueryParam("shift_id"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT r.id, r.receipt_number, r.receipt_type, r.total_amount, r.payment_type,
		r.created_at::text, COALESCE(o.status,'?'), COALESCE(o.fiscal_sign,'')
		FROM sales_receipt r LEFT JOIN ofd_send_status o ON o.receipt_id=r.id`
	var args []interface{}
	if shiftID != 0 {
		q += ` WHERE r.shift_id=$1`
		args = append(args, shiftID)
	}
	q += ` ORDER BY r.id DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var num, rtype, pay, ts, ost, sign string
		var total float64
		_ = rows.Scan(&id, &num, &rtype, &total, &pay, &ts, &ost, &sign)
		out = append(out, map[string]interface{}{"id": id, "receipt_number": num, "receipt_type": rtype,
			"total_amount": total, "payment_type": pay, "created_at": ts, "ofd_status": ost, "fiscal_sign": sign})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Receipt) GetOfdSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	if orgID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id required"})
	}
	var provider, url *string
	var auto bool
	var maxRet, failFirst int
	var active bool
	err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT provider, api_url, auto_send_enabled, max_retries, fail_first_attempts, is_active
		FROM ofd_settings WHERE organization_id=$1 AND is_active LIMIT 1`, orgID).
		Scan(&provider, &url, &auto, &maxRet, &failFirst, &active)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no settings"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"provider": provider, "api_url": url,
		"auto_send_enabled": auto, "max_retries": maxRet, "fail_first_attempts": failFirst, "is_active": active})
}

func (h *Receipt) PatchOfdSettings(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	if orgID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id required"})
	}
	var raw map[string]interface{}
	if err := c.Bind(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if v, ok := raw["fail_first_attempts"].(float64); ok {
		_, _ = h.Store.PG.Exec(c.Request().Context(), `
			UPDATE ofd_settings SET fail_first_attempts=$1 WHERE organization_id=$2`, int(v), orgID)
	}
	if v, ok := raw["auto_send_enabled"].(bool); ok {
		_, _ = h.Store.PG.Exec(c.Request().Context(), `
			UPDATE ofd_settings SET auto_send_enabled=$1 WHERE organization_id=$2`, v, orgID)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
