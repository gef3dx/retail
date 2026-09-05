package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// StockService — контрагенты, склады, поступления, заказы, отгрузки.
type StockService struct {
	Store          *store.Store
	Counterparties repository.CounterpartyRepo
	Warehouses     repository.WarehouseRepo
	Docs           repository.StockDocRepo
	Orders         repository.OrderRepo
	Shipments      repository.ShipmentRepo
	Products       repository.ProductRepo
	Balance        repository.BalanceRepo
	Notify         repository.NotifyRepo
}

// --- Counterparties ---

type CreateCounterpartyInput struct {
	OrgID       int64   `json:"org_id"`
	INN         string  `json:"inn"`
	FullName    string  `json:"full_name"`
	Phone       string  `json:"phone"`
	IsSupplier  bool    `json:"is_supplier"`
	IsBuyer     bool    `json:"is_buyer"`
	CreditLimit float64 `json:"credit_limit"`
}

func (s *StockService) ListCounterparties(ctx context.Context, orgID int64, role string) []model.Counterparty {
	return s.Counterparties.List(ctx, s.Store.PG, orgID, role)
}

func (s *StockService) CreateCounterparty(ctx context.Context, in CreateCounterpartyInput) (int64, error) {
	if in.OrgID == 0 || in.INN == "" || in.FullName == "" {
		return 0, BadRequest("org_id/inn/full_name required")
	}
	id, err := s.Counterparties.Create(ctx, s.Store.PG, in.OrgID, in.INN, in.FullName, in.Phone, in.IsSupplier, in.IsBuyer, in.CreditLimit)
	if err != nil {
		return 0, Conflict("duplicate inn in org")
	}
	return id, nil
}

// --- Warehouses ---

type CreateWarehouseInput struct {
	OrgID   int64  `json:"org_id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Type    string `json:"warehouse_type"`
}

func (s *StockService) ListWarehouses(ctx context.Context, orgID int64) []model.Warehouse {
	return s.Warehouses.List(ctx, s.Store.PG, orgID)
}

func (s *StockService) CreateWarehouse(ctx context.Context, in CreateWarehouseInput) (int64, error) {
	if in.OrgID == 0 || in.Code == "" || in.Name == "" {
		return 0, BadRequest("org_id/code/name required")
	}
	if in.Type == "" {
		in.Type = "MAIN"
	}
	id, err := s.Warehouses.Create(ctx, s.Store.PG, in.OrgID, in.Code, in.Name, in.Address, in.Type)
	if err != nil {
		return 0, Conflict("duplicate code in org")
	}
	return id, nil
}

// --- Balances ---

func (s *StockService) Balances(ctx context.Context, warehouseID int64) ([]model.Balance, error) {
	if warehouseID == 0 {
		return nil, BadRequest("warehouse_id required")
	}
	return s.Docs.Balances(ctx, s.Store.PG, warehouseID), nil
}

// --- Receipt docs ---

type ReceiptDocLineInput struct {
	ProductID int64    `json:"product_id"`
	Quantity  float64  `json:"quantity"`
	Price     float64  `json:"price"`
	VATRate   *float64 `json:"vat_rate"`
}

type CreateReceiptDocInput struct {
	WarehouseID int64                 `json:"warehouse_id"`
	SupplierID  int64                 `json:"supplier_id"`
	Number      string                `json:"number"`
	Lines       []ReceiptDocLineInput `json:"lines"`
	Comment     string                `json:"comment"`
}

func (s *StockService) CreateReceiptDoc(ctx context.Context, in CreateReceiptDocInput, userID int64) (int64, error) {
	if in.WarehouseID == 0 || in.SupplierID == 0 || len(in.Lines) == 0 {
		return 0, BadRequest("warehouse_id/supplier_id/lines required")
	}
	var id int64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		org, err := s.Warehouses.OrgOf(ctx, tx, in.WarehouseID)
		if err != nil {
			return NotFound("no warehouse")
		}
		number := in.Number
		if number == "" {
			number = s.Docs.NextDocNumber(ctx, tx, "receipt_document", org, "ПН-")
		}
		total, vat := 0.0, 0.0
		var lines []repository.ReceiptLineInput
		for _, l := range in.Lines {
			if l.Quantity <= 0 || l.Price < 0 {
				return BadRequest("bad line qty/price")
			}
			t := model.Round2(l.Price * l.Quantity)
			total += t
			vr := 20.0
			if l.VATRate != nil {
				vr = *l.VATRate
			}
			vat += model.Round2(t * vr / 100)
			lines = append(lines, repository.ReceiptLineInput{ProductID: l.ProductID, Quantity: l.Quantity, Price: l.Price, VATRate: vr})
		}
		id, err = s.Docs.CreateReceipt(ctx, tx, org, in.SupplierID, in.WarehouseID, number,
			model.Round2(total), model.Round2(vat), userID, in.Comment, lines)
		return err
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, se
		}
		return 0, Conflict("create receipt failed")
	}
	return id, nil
}

func (s *StockService) PostReceiptDoc(ctx context.Context, id int64) error {
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		wh, org, number, whName, items, totalQty, err := s.Docs.ReceiptForPost(ctx, tx, id)
		if err != nil {
			if err == repository.ErrAlreadyPosted {
				return Conflict("already posted")
			}
			return NotFound("no document")
		}
		if err := s.Balance.Add(ctx, tx, wh, items); err != nil {
			return Conflict("post failed")
		}
		s.Docs.MarkPosted(ctx, tx, id)
		data := map[string]interface{}{"doc_number": number, "total_qty": totalQty, "warehouse": whName}
		for _, uid := range s.Notify.Admins(ctx, tx, org) {
			s.Notify.EnqueueTx(ctx, tx, org, "STOCK_ARRIVED", []string{"WEB"},
				s.Notify.RecipientOf(ctx, tx, uid), "", "", data, "receipt_doc", &id, 5)
		}
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return se
		}
		return Conflict("post failed")
	}
	return nil
}

func (s *StockService) ListReceiptDocs(ctx context.Context, warehouseID int64) []model.StockReceipt {
	return s.Docs.ListReceipts(ctx, s.Store.PG, warehouseID)
}

// --- Orders ---

type OrderLineInput struct {
	ProductID int64    `json:"product_id"`
	Quantity  float64  `json:"quantity"`
	Price     *float64 `json:"price"`
}

type CreateOrderInput struct {
	WarehouseID int64            `json:"warehouse_id"`
	BuyerID     int64            `json:"buyer_id"`
	Number      string           `json:"number"`
	Type        string           `json:"order_type"`
	Lines       []OrderLineInput `json:"lines"`
}

func (s *StockService) CreateOrder(ctx context.Context, in CreateOrderInput, userID int64) (int64, error) {
	if in.WarehouseID == 0 || in.BuyerID == 0 || len(in.Lines) == 0 {
		return 0, BadRequest("warehouse_id/buyer_id/lines required")
	}
	if in.Type == "" {
		in.Type = "RETAIL"
	}
	var id int64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		org, err := s.Warehouses.OrgOf(ctx, tx, in.WarehouseID)
		if err != nil {
			return NotFound("no warehouse")
		}
		number := in.Number
		if number == "" {
			number = s.Docs.NextDocNumber(ctx, tx, "sales_order", org, "ЗК-")
		}
		total, vat := 0.0, 0.0
		var lines []repository.OrderLineData
		for _, l := range in.Lines {
			if l.Quantity <= 0 {
				return BadRequest("bad qty")
			}
			price := 0.0
			if l.Price != nil {
				price = *l.Price
			} else {
				rp := s.Products.RetailPrice(ctx, tx, l.ProductID, org)
				if rp == nil {
					return BadRequest("no retail price for product")
				}
				price = *rp
			}
			var vr float64
			if err := tx.QueryRow(ctx, `SELECT vat_rate FROM catalog_product WHERE id=$1`, l.ProductID).Scan(&vr); err != nil {
				return NotFound("no product")
			}
			t := model.Round2(price * l.Quantity)
			total += t
			vat += model.Round2(t * vr / 100)
			lines = append(lines, repository.OrderLineData{Product: l.ProductID, Quantity: l.Quantity, Price: price, VAT: vr})
		}
		id, err = s.Orders.Create(ctx, tx, org, in.BuyerID, in.WarehouseID, number, in.Type,
			model.Round2(total), model.Round2(vat), userID, lines)
		if err != nil {
			return Conflict("create order failed")
		}
		s.Notify.EnqueueTx(ctx, tx, org, "ORDER_CREATED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, userID), "", "",
			map[string]interface{}{"order_number": number,
				"order_date":   time.Now().Format("2006-01-02"),
				"total_amount": model.Round2(total)}, "order", &id, 5)
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, se
		}
		return 0, Conflict("create order failed")
	}
	return id, nil
}

func (s *StockService) ListOrders(ctx context.Context, status string) []model.Order {
	return s.Orders.List(ctx, s.Store.PG, status)
}

func (s *StockService) GetOrder(ctx context.Context, id int64) (model.OrderDetail, error) {
	d, err := s.Orders.Detail(ctx, s.Store.PG, id)
	if err != nil {
		return d, NotFound("no order")
	}
	if d.Lines == nil {
		d.Lines = []model.OrderLine{}
	}
	return d, nil
}

func (s *StockService) ConfirmOrder(ctx context.Context, id int64) error {
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		h, err := s.Orders.HeadForUpdate(ctx, tx, id)
		if err != nil {
			return NotFound("no order")
		}
		if h.Status != "DRAFT" {
			return Conflict("only DRAFT can be confirmed")
		}
		lines := s.Orders.Lines(ctx, tx, id)
		items := map[int64]float64{}
		for _, l := range lines {
			items[l.Product] += l.Quantity
		}
		if err := s.Balance.Reserve(ctx, tx, h.Warehouse, items); err != nil {
			if err == repository.ErrInsufficientStock {
				return Conflict("insufficient stock to reserve")
			}
			return Conflict("confirm failed")
		}
		for _, l := range lines {
			s.Orders.SetLineReserved(ctx, tx, l.ID, l.Quantity)
		}
		s.Orders.SetStatus(ctx, tx, id, "CONFIRMED")
		s.Notify.EnqueueTx(ctx, tx, h.Org, "ORDER_STATUS_CHANGED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, h.Manager), "", "",
			map[string]interface{}{"order_number": h.Number, "new_status": "CONFIRMED"}, "order", &id, 5)
		s.Notify.CheckLowStock(ctx, tx, h.Org, h.Warehouse)
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return se
		}
		return Conflict("confirm failed")
	}
	return nil
}

func (s *StockService) CancelOrder(ctx context.Context, id int64) error {
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		h, err := s.Orders.HeadForUpdate(ctx, tx, id)
		if err != nil {
			return NotFound("no order")
		}
		if h.Status == "CANCELED" || h.Status == "COMPLETED" {
			return Conflict("cannot cancel " + h.Status)
		}
		if h.Status == "CONFIRMED" {
			if err := s.Balance.Release(ctx, tx, h.Warehouse, s.Orders.LinesReserved(ctx, tx, id)); err != nil {
				return Conflict("cancel failed")
			}
		}
		s.Orders.SetStatus(ctx, tx, id, "CANCELED")
		s.Notify.EnqueueTx(ctx, tx, h.Org, "ORDER_STATUS_CHANGED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, h.Manager), "", "",
			map[string]interface{}{"order_number": h.Number, "new_status": "CANCELED"}, "order", &id, 5)
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return se
		}
		return Conflict("cancel failed")
	}
	return nil
}

// --- Shipments ---

type ShipLineInput struct {
	OrderLineID int64   `json:"order_line_id"`
	Quantity    float64 `json:"quantity"`
}

type CreateShipmentInput struct {
	OrderID int64           `json:"order_id"`
	Lines   []ShipLineInput `json:"lines"`
	Number  string          `json:"number"`
}

func (s *StockService) CreateShipment(ctx context.Context, in CreateShipmentInput, userID int64) (int64, error) {
	if in.OrderID == 0 {
		return 0, BadRequest("order_id required")
	}
	var sid int64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		h, err := s.Orders.HeadForUpdate(ctx, tx, in.OrderID)
		if err != nil {
			return NotFound("no order")
		}
		if h.Status != "CONFIRMED" && h.Status != "SHIPPED" {
			return Conflict("order must be CONFIRMED")
		}
		ols := s.Orders.Lines(ctx, tx, in.OrderID)
		remain := map[int64]repository.OrderLineData{}
		for _, l := range ols {
			var shipped float64
			_ = tx.QueryRow(ctx, `
				SELECT COALESCE(SUM(s.quantity),0) FROM shipment_line s
				JOIN shipment_document d ON d.id=s.document_id
				WHERE s.order_line_id=$1 AND d.is_posted`, l.ID).Scan(&shipped)
			l.Quantity = l.Quantity - shipped
			remain[l.ID] = l
		}
		toShip := map[int64]float64{}
		if len(in.Lines) == 0 {
			for lid, o := range remain {
				if o.Quantity > 0 {
					toShip[lid] = o.Quantity
				}
			}
		} else {
			for _, l := range in.Lines {
				o, ok := remain[l.OrderLineID]
				if !ok || l.Quantity <= 0 || l.Quantity > o.Quantity {
					return BadRequest("bad ship line qty")
				}
				toShip[l.OrderLineID] = l.Quantity
			}
		}
		if len(toShip) == 0 {
			return Conflict("nothing to ship")
		}
		number := in.Number
		if number == "" {
			number = s.Docs.NextDocNumber(ctx, tx, "shipment_document", h.Org, "ОТ-")
		}
		total, vat := 0.0, 0.0
		ity := map[int64]float64{}
		for lid, qty := range toShip {
			o := remain[lid]
			t := model.Round2(o.Price * qty)
			total += t
			vat += model.Round2(t * o.VAT / 100)
			ity[o.Product] += qty
		}
		sid, err = s.Shipments.Create(ctx, tx, h.Org, h.Buyer, h.Warehouse, &in.OrderID,
			number, model.Round2(total), model.Round2(vat), userID)
		if err != nil {
			return Conflict("shipment failed")
		}
		for lid, qty := range toShip {
			o := remain[lid]
			if err := s.Shipments.AddLine(ctx, tx, sid, o.Product, &lid, qty, o.Price, o.VAT); err != nil {
				return Conflict("shipment failed")
			}
		}
		if err := s.Balance.Ship(ctx, tx, h.Warehouse, ity); err != nil {
			if err == repository.ErrInsufficientStock {
				return Conflict("insufficient stock to ship")
			}
			return Conflict("shipment failed")
		}
		ns := "SHIPPED"
		if s.Orders.RemainingTotal(ctx, tx, in.OrderID) <= 0 {
			ns = "COMPLETED"
		}
		s.Orders.SetStatus(ctx, tx, in.OrderID, ns)
		s.Notify.EnqueueTx(ctx, tx, h.Org, "ORDER_STATUS_CHANGED", []string{"WEB", "EMAIL"},
			s.Notify.RecipientOf(ctx, tx, h.Manager), "", "",
			map[string]interface{}{"order_number": h.Number, "new_status": ns}, "order", &in.OrderID, 5)
		s.Notify.CheckLowStock(ctx, tx, h.Org, h.Warehouse)
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, se
		}
		return 0, Conflict("shipment failed")
	}
	return sid, nil
}

func (s *StockService) ListShipments(ctx context.Context, orderID int64) []model.Shipment {
	return s.Shipments.List(ctx, s.Store.PG, orderID)
}
