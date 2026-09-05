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

type Stock struct {
	Store *store.Store
}

// ---------- Counterparties ----------

func (h *Stock) ListCounterparties(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	role := c.QueryParam("role") // supplier | buyer
	q := `SELECT id, inn, full_name, phone, is_supplier, is_buyer, credit_limit FROM counterparty WHERE organization_id=$1 AND is_active`
	args := []interface{}{orgID}
	if role == "supplier" {
		q += ` AND is_supplier=TRUE`
	} else if role == "buyer" {
		q += ` AND is_buyer=TRUE`
	}
	q += ` ORDER BY id`
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var inn, name string
		var phone *string
		var sup, buy bool
		var limit float64
		_ = rows.Scan(&id, &inn, &name, &phone, &sup, &buy, &limit)
		out = append(out, map[string]interface{}{"id": id, "inn": inn, "full_name": name,
			"phone": phone, "is_supplier": sup, "is_buyer": buy, "credit_limit": limit})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Stock) CreateCounterparty(c echo.Context) error {
	var b struct {
		OrgID      int64   `json:"org_id"`
		INN        string  `json:"inn"`
		FullName   string  `json:"full_name"`
		Phone      string  `json:"phone"`
		IsSupplier bool    `json:"is_supplier"`
		IsBuyer    bool    `json:"is_buyer"`
		CreditLimit float64 `json:"credit_limit"`
	}
	if err := c.Bind(&b); err != nil || b.OrgID == 0 || b.INN == "" || b.FullName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id/inn/full_name required"})
	}
	var id int64
	if err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO counterparty(organization_id, inn, full_name, phone, is_supplier, is_buyer, credit_limit)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7) RETURNING id`,
		b.OrgID, b.INN, b.FullName, b.Phone, b.IsSupplier, b.IsBuyer, b.CreditLimit).Scan(&id); err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "duplicate inn in org"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

// ---------- Warehouses ----------

func (h *Stock) ListWarehouses(c echo.Context) error {
	orgID, _ := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT id, code, name, warehouse_type FROM warehouse
		WHERE organization_id=$1 AND is_active ORDER BY id`, orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var code, name, wtype string
		_ = rows.Scan(&id, &code, &name, &wtype)
		out = append(out, map[string]interface{}{"id": id, "code": code, "name": name, "warehouse_type": wtype})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Stock) CreateWarehouse(c echo.Context) error {
	var b struct {
		OrgID   int64  `json:"org_id"`
		Code    string `json:"code"`
		Name    string `json:"name"`
		Address string `json:"address"`
		Type    string `json:"warehouse_type"`
	}
	if err := c.Bind(&b); err != nil || b.OrgID == 0 || b.Code == "" || b.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id/code/name required"})
	}
	if b.Type == "" {
		b.Type = "MAIN"
	}
	var id int64
	if err := h.Store.PG.QueryRow(c.Request().Context(), `
		INSERT INTO warehouse(organization_id, code, name, address, warehouse_type)
		VALUES($1,$2,$3,$4,$5) RETURNING id`, b.OrgID, b.Code, b.Name, b.Address, b.Type).Scan(&id); err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "duplicate code in org"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

// ---------- Balances ----------

func (h *Stock) Balances(c echo.Context) error {
	whID, _ := strconv.ParseInt(c.QueryParam("warehouse_id"), 10, 64)
	if whID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "warehouse_id required"})
	}
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT b.product_id, p.sku, p.name, b.quantity, b.reserved_quantity,
		       (b.quantity - b.reserved_quantity) AS available
		FROM warehouse_balance b JOIN catalog_product p ON p.id=b.product_id
		WHERE b.warehouse_id=$1 ORDER BY p.name`, whID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var pid int64
		var sku, name string
		var qty, res, avail float64
		_ = rows.Scan(&pid, &sku, &name, &qty, &res, &avail)
		out = append(out, map[string]interface{}{"product_id": pid, "sku": sku, "name": name,
			"quantity": qty, "reserved": res, "available": avail})
	}
	return c.JSON(http.StatusOK, out)
}

// deductLocked списывает свободные остатки (qty - reserved). Вызывать в транзакции.
func deductLocked(tx pgx.Tx, ctx context.Context, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		var avail float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(quantity,0) - COALESCE(reserved_quantity,0) FROM warehouse_balance
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid).Scan(&avail); err != nil {
			return echo.NewHTTPError(http.StatusConflict, "no stock for product")
		}
		if avail < qty {
			return echo.NewHTTPError(http.StatusConflict, "insufficient stock")
		}
		if _, err := tx.Exec(ctx, `
			UPDATE warehouse_balance SET quantity = quantity - $3, last_updated=NOW()
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid, qty); err != nil {
			return err
		}
	}
	return nil
}

func addLocked(tx pgx.Tx, ctx context.Context, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO warehouse_balance(warehouse_id, product_id, quantity)
			VALUES($1,$2,$3)
			ON CONFLICT (warehouse_id, product_id) DO UPDATE SET quantity = warehouse_balance.quantity + $3, last_updated=NOW()`,
			warehouseID, pid, qty); err != nil {
			return err
		}
	}
	return nil
}

func reserveLocked(tx pgx.Tx, ctx context.Context, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		res, err := tx.Exec(ctx, `
			UPDATE warehouse_balance SET reserved_quantity = reserved_quantity + $3, last_updated=NOW()
			WHERE warehouse_id=$1 AND product_id=$2
			  AND (quantity - reserved_quantity) >= $3`, warehouseID, pid, qty)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			// Нет строки остатков — создать нулевую не поможет (доступно 0). Ошибка.
			return echo.NewHTTPError(http.StatusConflict, "insufficient stock to reserve")
		}
	}
	return nil
}

func releaseLocked(tx pgx.Tx, ctx context.Context, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		if _, err := tx.Exec(ctx, `
			UPDATE warehouse_balance SET reserved_quantity = GREATEST(0, reserved_quantity - $3), last_updated=NOW()
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid, qty); err != nil {
			return err
		}
	}
	return nil
}

// ---------- Receipt documents (поступления) ----------

type receiptDocReq struct {
	WarehouseID int64 `json:"warehouse_id"`
	SupplierID  int64 `json:"supplier_id"`
	Number      string `json:"number"`
	Lines       []struct {
		ProductID int64   `json:"product_id"`
		Quantity  float64 `json:"quantity"`
		Price     float64 `json:"price"`
		VATRate   *float64 `json:"vat_rate"`
	} `json:"lines"`
	Comment string `json:"comment"`
}

func (h *Stock) CreateReceiptDoc(c echo.Context) error {
	var b receiptDocReq
	if err := c.Bind(&b); err != nil || b.WarehouseID == 0 || b.SupplierID == 0 || len(b.Lines) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "warehouse_id/supplier_id/lines required"})
	}
	x := middleware.CtxOf(c)
	var id int64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var org int64
		if err := tx.QueryRow(ctx, `SELECT organization_id FROM warehouse WHERE id=$1`, b.WarehouseID).Scan(&org); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no warehouse")
		}
		number := b.Number
		if number == "" {
			var n int64
			_ = tx.QueryRow(ctx, `SELECT COUNT(*)+1 FROM receipt_document WHERE organization_id=$1`, org).Scan(&n)
			number = "ПН-" + strconv.FormatInt(n, 10)
		}
		total, vat := 0.0, 0.0
		for _, l := range b.Lines {
			if l.Quantity <= 0 || l.Price < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "bad line qty/price")
			}
			t := round2(l.Price * l.Quantity)
			total += t
			vr := 20.0
			if l.VATRate != nil {
				vr = *l.VATRate
			}
			vat += round2(t * vr / 100)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO receipt_document(organization_id, supplier_id, warehouse_id, document_number,
				total_amount, total_vat, responsible_id, comment)
			VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')) RETURNING id`,
			org, b.SupplierID, b.WarehouseID, number, round2(total), round2(vat), x.UserID, b.Comment).Scan(&id); err != nil {
			return err
		}
		for _, l := range b.Lines {
			vr := 20.0
			if l.VATRate != nil {
				vr = *l.VATRate
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO receipt_line(document_id, product_id, quantity, price, vat_rate)
				VALUES($1,$2,$3,$4,$5)`, id, l.ProductID, l.Quantity, l.Price, vr); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "create receipt failed"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Stock) PostReceiptDoc(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var wh, org int64
		var posted bool
		var number, whName string
		if err := tx.QueryRow(ctx, `
			SELECT d.warehouse_id, d.is_posted, d.organization_id, d.document_number, w.name
			FROM receipt_document d JOIN warehouse w ON w.id=d.warehouse_id
			WHERE d.id=$1 FOR UPDATE OF d`, id).
			Scan(&wh, &posted, &org, &number, &whName); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no document")
		}
		if posted {
			return echo.NewHTTPError(http.StatusConflict, "already posted")
		}
		rows, err := tx.Query(ctx, `SELECT product_id, quantity FROM receipt_line WHERE document_id=$1`, id)
		if err != nil {
			return err
		}
		items := map[int64]float64{}
		totalQty := 0.0
		for rows.Next() {
			var pid int64
			var qty float64
			_ = rows.Scan(&pid, &qty)
			items[pid] += qty
			totalQty += qty
		}
		rows.Close()
		if err := addLocked(tx, ctx, wh, items); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `UPDATE receipt_document SET is_posted=TRUE, posted_at=NOW() WHERE id=$1`, id)
		data := map[string]interface{}{"doc_number": number, "total_qty": totalQty, "warehouse": whName}
		for _, uid := range notify.Admins(ctx, h.Store, org) {
			notify.EnqueueTx(tx, ctx, org, "STOCK_ARRIVED", []string{"WEB"},
				notify.RecipientOf(ctx, h.Store, uid), "", "", data, "receipt_doc", &id, 5)
		}
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "post failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "posted"})
}

func (h *Stock) ListReceiptDocs(c echo.Context) error {
	whID, _ := strconv.ParseInt(c.QueryParam("warehouse_id"), 10, 64)
	q := `SELECT id, document_number, document_date::text, total_amount, is_posted FROM receipt_document`
	var args []interface{}
	if whID != 0 {
		q += ` WHERE warehouse_id=$1`
		args = append(args, whID)
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
		var num, dt string
		var total float64
		var posted bool
		_ = rows.Scan(&id, &num, &dt, &total, &posted)
		out = append(out, map[string]interface{}{"id": id, "number": num, "date": dt, "total": total, "posted": posted})
	}
	return c.JSON(http.StatusOK, out)
}

// ---------- Orders ----------

type orderReq struct {
	WarehouseID int64 `json:"warehouse_id"`
	BuyerID     int64 `json:"buyer_id"`
	Number      string `json:"number"`
	Type        string `json:"order_type"`
	Lines       []struct {
		ProductID int64    `json:"product_id"`
		Quantity  float64  `json:"quantity"`
		Price     *float64 `json:"price"`
	} `json:"lines"`
}

func (h *Stock) CreateOrder(c echo.Context) error {
	var b orderReq
	if err := c.Bind(&b); err != nil || b.WarehouseID == 0 || b.BuyerID == 0 || len(b.Lines) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "warehouse_id/buyer_id/lines required"})
	}
	if b.Type == "" {
		b.Type = "RETAIL"
	}
	x := middleware.CtxOf(c)
	var id int64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var org int64
		if err := tx.QueryRow(ctx, `SELECT organization_id FROM warehouse WHERE id=$1`, b.WarehouseID).Scan(&org); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no warehouse")
		}
		number := b.Number
		if number == "" {
			var n int64
			_ = tx.QueryRow(ctx, `SELECT COUNT(*)+1 FROM sales_order WHERE organization_id=$1`, org).Scan(&n)
			number = "ЗК-" + strconv.FormatInt(n, 10)
		}
		total, vat := 0.0, 0.0
		type ln struct {
			pid, qty, price, vr float64
		}
		var lines []ln
		for _, l := range b.Lines {
			if l.Quantity <= 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "bad qty")
			}
			price := 0.0
			if l.Price != nil {
				price = *l.Price
			} else {
				var rp *float64
				_ = tx.QueryRow(ctx, `
					SELECT pp.price FROM product_price pp JOIN price_type pt ON pt.id=pp.price_type_id
					WHERE pp.product_id=$1 AND pt.organization_id=$2 AND pt.price_kind='RETAIL'
					  AND pp.valid_from <= CURRENT_DATE AND (pp.valid_to IS NULL OR pp.valid_to >= CURRENT_DATE)
					ORDER BY pp.valid_from DESC LIMIT 1`, l.ProductID, org).Scan(&rp)
				if rp == nil {
					return echo.NewHTTPError(http.StatusBadRequest, "no retail price for product")
				}
				price = *rp
			}
			var vr float64
			if err := tx.QueryRow(ctx, `SELECT vat_rate FROM catalog_product WHERE id=$1`, l.ProductID).Scan(&vr); err != nil {
				return echo.NewHTTPError(http.StatusNotFound, "no product")
			}
			t := round2(price * l.Quantity)
			total += t
			vat += round2(t * vr / 100)
			lines = append(lines, ln{float64(l.ProductID), l.Quantity, price, vr})
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO sales_order(organization_id, buyer_id, warehouse_id, order_number, order_type,
				total_amount, total_vat, manager_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
			org, b.BuyerID, b.WarehouseID, number, b.Type, round2(total), round2(vat), x.UserID).Scan(&id); err != nil {
			return err
		}
		for _, l := range lines {
			if _, err := tx.Exec(ctx, `
				INSERT INTO sales_order_line(order_id, product_id, quantity, price, vat_rate)
				VALUES($1,$2,$3,$4,$5)`, id, int64(l.pid), l.qty, l.price, l.vr); err != nil {
				return err
			}
		}
		notify.EnqueueTx(tx, ctx, org, "ORDER_CREATED", []string{"WEB", "EMAIL"},
			notify.RecipientOf(ctx, h.Store, x.UserID), "", "",
			map[string]interface{}{"order_number": number,
				"order_date": time.Now().Format("2006-01-02"),
				"total_amount": round2(total)}, "order", &id, 5)
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "create order failed"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

func (h *Stock) ListOrders(c echo.Context) error {
	status := c.QueryParam("status")
	q := `SELECT o.id, o.order_number, o.order_type, o.total_amount, o.status, cp.full_name
		FROM sales_order o JOIN counterparty cp ON cp.id=o.buyer_id`
	var args []interface{}
	if status != "" {
		q += ` WHERE o.status=$1`
		args = append(args, status)
	}
	q += ` ORDER BY o.id DESC LIMIT 100`
	rows, err := h.Store.PG.Query(c.Request().Context(), q, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var num, otype, st, buyer string
		var total float64
		_ = rows.Scan(&id, &num, &otype, &total, &st, &buyer)
		out = append(out, map[string]interface{}{"id": id, "number": num, "type": otype,
			"total": total, "status": st, "buyer": buyer})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Stock) GetOrder(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var num, st string
	var total float64
	var wh int64
	if err := h.Store.PG.QueryRow(c.Request().Context(), `
		SELECT order_number, status, total_amount, warehouse_id FROM sales_order WHERE id=$1`, id).
		Scan(&num, &st, &total, &wh); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no order"})
	}
	rows, err := h.Store.PG.Query(c.Request().Context(), `
		SELECT l.id, l.product_id, p.name, l.quantity, l.price, l.reserved_quantity,
		       COALESCE((SELECT SUM(s.quantity) FROM shipment_line s
		                 JOIN shipment_document d ON d.id=s.document_id
		                 WHERE s.order_line_id=l.id AND d.is_posted),0) AS shipped
		FROM sales_order_line l JOIN catalog_product p ON p.id=l.product_id
		WHERE l.order_id=$1 ORDER BY l.id`, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
	}
	defer rows.Close()
	var lines []map[string]interface{}
	for rows.Next() {
		var lid, pid int64
		var name string
		var qty, price, res, shipped float64
		_ = rows.Scan(&lid, &pid, &name, &qty, &price, &res, &shipped)
		lines = append(lines, map[string]interface{}{"id": lid, "product_id": pid, "name": name,
			"quantity": qty, "price": price, "reserved": res, "shipped": shipped})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": id, "number": num, "status": st,
		"total": total, "warehouse_id": wh, "lines": lines})
}

func (h *Stock) ConfirmOrder(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var wh, manager int64
		var st, number string
		if err := tx.QueryRow(ctx, `SELECT warehouse_id, manager_id, status, order_number FROM sales_order WHERE id=$1 FOR UPDATE`, id).
			Scan(&wh, &manager, &st, &number); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no order")
		}
		if st != "DRAFT" {
			return echo.NewHTTPError(http.StatusConflict, "only DRAFT can be confirmed")
		}
		rows, err := tx.Query(ctx, `SELECT id, product_id, quantity FROM sales_order_line WHERE order_id=$1`, id)
		if err != nil {
			return err
		}
		items := map[int64]float64{}
		type rl struct {
			lid, pid int64
			qty      float64
		}
		var rls []rl
		for rows.Next() {
			var r rl
			_ = rows.Scan(&r.lid, &r.pid, &r.qty)
			rls = append(rls, r)
			items[r.pid] += r.qty
		}
		rows.Close()
		if err := reserveLocked(tx, ctx, wh, items); err != nil {
			return err
		}
		for _, r := range rls {
			_, _ = tx.Exec(ctx, `UPDATE sales_order_line SET reserved_quantity=$2 WHERE id=$1`, r.lid, r.qty)
		}
		_, _ = tx.Exec(ctx, `UPDATE sales_order SET status='CONFIRMED' WHERE id=$1`, id)
		var org int64
		_ = tx.QueryRow(ctx, `SELECT organization_id FROM sales_order WHERE id=$1`, id).Scan(&org)
		notify.EnqueueTx(tx, ctx, org, "ORDER_STATUS_CHANGED", []string{"WEB", "EMAIL"},
			notify.RecipientOf(ctx, h.Store, manager), "", "",
			map[string]interface{}{"order_number": number, "new_status": "CONFIRMED"}, "order", &id, 5)
		notify.CheckLowStockTx(tx, ctx, h.Store, org, wh)
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "confirm failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "confirmed"})
}

func (h *Stock) CancelOrder(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var wh, manager int64
		var st, number string
		var org int64
		if err := tx.QueryRow(ctx, `SELECT warehouse_id, manager_id, status, order_number, organization_id FROM sales_order WHERE id=$1 FOR UPDATE`, id).
			Scan(&wh, &manager, &st, &number, &org); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no order")
		}
		if st == "CANCELED" || st == "COMPLETED" {
			return echo.NewHTTPError(http.StatusConflict, "cannot cancel "+st)
		}
		if st == "CONFIRMED" {
			rows, err := tx.Query(ctx, `SELECT product_id, reserved_quantity FROM sales_order_line WHERE order_id=$1`, id)
			if err != nil {
				return err
			}
			items := map[int64]float64{}
			for rows.Next() {
				var pid int64
				var qty float64
				_ = rows.Scan(&pid, &qty)
				items[pid] += qty
			}
			rows.Close()
			if err := releaseLocked(tx, ctx, wh, items); err != nil {
				return err
			}
		}
		_, _ = tx.Exec(ctx, `UPDATE sales_order SET status='CANCELED' WHERE id=$1`, id)
		notify.EnqueueTx(tx, ctx, org, "ORDER_STATUS_CHANGED", []string{"WEB", "EMAIL"},
			notify.RecipientOf(ctx, h.Store, manager), "", "",
			map[string]interface{}{"order_number": number, "new_status": "CANCELED"}, "order", &id, 5)
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "cancel failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "canceled"})
}

// ---------- Shipments ----------

type shipReq struct {
	OrderID int64 `json:"order_id"`
	Lines   []struct {
		OrderLineID int64   `json:"order_line_id"`
		Quantity    float64 `json:"quantity"`
	} `json:"lines"`
	Number string `json:"number"`
}

func (h *Stock) CreateShipment(c echo.Context) error {
	var b shipReq
	if err := c.Bind(&b); err != nil || b.OrderID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "order_id required"})
	}
	x := middleware.CtxOf(c)
	var sid int64
	err := h.Store.Tx(c.Request().Context(), func(tx pgx.Tx) error {
		ctx := c.Request().Context()
		var org, wh, buyer int64
		var st string
		if err := tx.QueryRow(ctx, `
			SELECT organization_id, warehouse_id, buyer_id, status FROM sales_order WHERE id=$1 FOR UPDATE`, b.OrderID).
			Scan(&org, &wh, &buyer, &st); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "no order")
		}
		if st != "CONFIRMED" && st != "SHIPPED" {
			return echo.NewHTTPError(http.StatusConflict, "order must be CONFIRMED")
		}
		// Строки заказа + уже отгруженное.
		rows, err := tx.Query(ctx, `
			SELECT l.id, l.product_id, l.quantity, l.price, l.vat_rate,
			       COALESCE((SELECT SUM(s.quantity) FROM shipment_line s
			                 JOIN shipment_document d ON d.id=s.document_id
			                 WHERE s.order_line_id=l.id AND d.is_posted),0) AS shipped
			FROM sales_order_line l WHERE l.order_id=$1`, b.OrderID)
		if err != nil {
			return err
		}
		type ol struct {
			lid, pid       int64
			qty, price, vr float64
			remain         float64
		}
		varols := map[int64]ol{}
		for rows.Next() {
			var o ol
			var shipped float64
			_ = rows.Scan(&o.lid, &o.pid, &o.qty, &o.price, &o.vr, &shipped)
			o.remain = o.qty - shipped
			varols[o.lid] = o
		}
		rows.Close()
		// Какие строки отгружаем: явно указанные или все остатки.
		toShip := map[int64]float64{}
		if len(b.Lines) == 0 {
			for lid, o := range varols {
				if o.remain > 0 {
					toShip[lid] = o.remain
				}
			}
		} else {
			for _, l := range b.Lines {
				o, ok := varols[l.OrderLineID]
				if !ok || l.Quantity <= 0 || l.Quantity > o.remain {
					return echo.NewHTTPError(http.StatusBadRequest, "bad ship line qty")
				}
				toShip[l.OrderLineID] = l.Quantity
			}
		}
		if len(toShip) == 0 {
			return echo.NewHTTPError(http.StatusConflict, "nothing to ship")
		}
		number := b.Number
		if number == "" {
			var n int64
			_ = tx.QueryRow(ctx, `SELECT COUNT(*)+1 FROM shipment_document WHERE organization_id=$1`, org).Scan(&n)
			number = "ОТ-" + strconv.FormatInt(n, 10)
		}
		total, vat, ity := 0.0, 0.0, map[int64]float64{}
		for lid, qty := range toShip {
			o := varols[lid]
			t := round2(o.price * qty)
			total += t
			vat += round2(t * o.vr / 100)
			ity[o.pid] += qty
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO shipment_document(organization_id, buyer_id, warehouse_id, sales_order_id,
				document_number, total_amount, total_vat, is_posted, posted_at, responsible_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,TRUE,NOW(),$8) RETURNING id`,
			org, buyer, wh, b.OrderID, number, round2(total), round2(vat), x.UserID).Scan(&sid); err != nil {
			return err
		}
		// Списываем остатки И снимаем резерв (резерв был под заказ).
		for lid, qty := range toShip {
			o := varols[lid]
			if _, err := tx.Exec(ctx, `
				INSERT INTO shipment_line(document_id, product_id, order_line_id, quantity, price, vat_rate)
				VALUES($1,$2,$3,$4,$5,$6)`, sid, o.pid, lid, qty, o.price, o.vr); err != nil {
				return err
			}
		}
		for pid, qty := range ity {
			var physical float64
			if err := tx.QueryRow(ctx, `
				SELECT quantity FROM warehouse_balance
				WHERE warehouse_id=$1 AND product_id=$2`, wh, pid).Scan(&physical); err != nil || physical < qty {
				return echo.NewHTTPError(http.StatusConflict, "insufficient stock to ship")
			}
			// Резерв был под этот заказ — снимаем его вместе со списанием.
			if _, err := tx.Exec(ctx, `
				UPDATE warehouse_balance SET quantity = quantity - $3,
					reserved_quantity = GREATEST(0, reserved_quantity - $3), last_updated=NOW()
				WHERE warehouse_id=$1 AND product_id=$2`, wh, pid, qty); err != nil {
				return err
			}
		}
		// Статус заказа: все отгружено → COMPLETED, иначе SHIPPED.
		var left float64
		_ = tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(l.quantity - COALESCE((SELECT SUM(s.quantity) FROM shipment_line s
				JOIN shipment_document d ON d.id=s.document_id
				WHERE s.order_line_id=l.id AND d.is_posted),0)),0)
			FROM sales_order_line l WHERE l.order_id=$1`, b.OrderID).Scan(&left)
		ns := "SHIPPED"
		if left <= 0 {
			ns = "COMPLETED"
		}
		_, _ = tx.Exec(ctx, `UPDATE sales_order SET status=$2 WHERE id=$1`, b.OrderID, ns)
		var manager int64
		var orderNumber string
		_ = tx.QueryRow(ctx, `SELECT manager_id, order_number FROM sales_order WHERE id=$1`, b.OrderID).Scan(&manager, &orderNumber)
		notify.EnqueueTx(tx, ctx, org, "ORDER_STATUS_CHANGED", []string{"WEB", "EMAIL"},
			notify.RecipientOf(ctx, h.Store, manager), "", "",
			map[string]interface{}{"order_number": orderNumber, "new_status": ns}, "order", &b.OrderID, 5)
		notify.CheckLowStockTx(tx, ctx, h.Store, org, wh)
		return nil
	})
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return c.JSON(he.Code, map[string]string{"error": he.Message.(string)})
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": "shipment failed"})
	}
	return c.JSON(http.StatusCreated, map[string]int64{"id": sid})
}

func (h *Stock) ListShipments(c echo.Context) error {
	orderID, _ := strconv.ParseInt(c.QueryParam("order_id"), 10, 64)
	q := `SELECT id, document_number, total_amount, is_posted, sales_order_id FROM shipment_document`
	var args []interface{}
	if orderID != 0 {
		q += ` WHERE sales_order_id=$1`
		args = append(args, orderID)
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
		var num string
		var total float64
		var posted bool
		var oid *int64
		_ = rows.Scan(&id, &num, &total, &posted, &oid)
		out = append(out, map[string]interface{}{"id": id, "number": num, "total": total, "posted": posted, "order_id": oid})
	}
	return c.JSON(http.StatusOK, out)
}
