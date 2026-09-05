package service

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// ReceiptService — кассы, смены, чеки.
type ReceiptService struct {
	Store     *store.Store
	Registers repository.RegisterRepo
	Shifts    repository.ShiftRepo
	Receipts  repository.ReceiptRepo
	Products  repository.ProductRepo
	Balances  repository.BalanceRepo
	Marking   repository.MarkingRepo
	Notify    repository.NotifyRepo
	Ofd       repository.OfdRepo
	Audit     repository.AuditRepo
}

// --- Registers ---

func (s *ReceiptService) ListRegisters(ctx context.Context, orgID int64) []model.Register {
	return s.Registers.List(ctx, s.Store.PG, orgID)
}

type CreateRegisterInput struct {
	OrganizationID int64  `json:"organization_id"`
	RegNumber      string `json:"reg_number"`
	Model          string `json:"model"`
	Address        string `json:"address"`
}

func (s *ReceiptService) CreateRegister(ctx context.Context, in CreateRegisterInput) (int64, error) {
	if in.OrganizationID == 0 || in.RegNumber == "" {
		return 0, BadRequest("organization_id/reg_number required")
	}
	id, err := s.Registers.Create(ctx, s.Store.PG, in.OrganizationID, in.RegNumber, in.Model, in.Address)
	if err != nil {
		return 0, Conflict("duplicate reg_number")
	}
	return id, nil
}

func (s *ReceiptService) PatchRegister(ctx context.Context, id int64, raw map[string]interface{}) error {
	var wh *int64
	hasWh := false
	if v, ok := raw["warehouse_id"].(float64); ok {
		hasWh = true
		if v != 0 {
			w := int64(v)
			wh = &w
		}
	}
	var status *string
	if v, ok := raw["status"].(string); ok && (v == "ACTIVE" || v == "INACTIVE" || v == "BLOCKED") {
		status = &v
	}
	if err := s.Registers.Patch(ctx, s.Store.PG, id, wh, hasWh, status); err != nil {
		return BadRequest(err.Error())
	}
	return nil
}

// --- Shifts ---

func (s *ReceiptService) OpenShift(ctx context.Context, regID int64, startCash float64, userID int64) (int64, int64, error) {
	if regID == 0 {
		return 0, 0, BadRequest("cash_register_id required")
	}
	id, number, err := s.Shifts.Open(ctx, s.Store.PG, regID, userID, startCash)
	if err != nil {
		return 0, 0, Conflict("open failed (shift already open?)")
	}
	return id, number, nil
}

func (s *ReceiptService) OpenShiftForRegister(ctx context.Context, regID int64) (int64, int64, error) {
	if regID == 0 {
		return 0, 0, BadRequest("register_id required")
	}
	id, number, err := s.Shifts.GetOpen(ctx, s.Store.PG, regID)
	if err != nil {
		return 0, 0, NotFound("no open shift")
	}
	return id, number, nil
}

func (s *ReceiptService) XReport(ctx context.Context, shiftID int64) (model.ShiftReport, error) {
	rep, err := s.Shifts.Report(ctx, s.Store.PG, shiftID)
	if err != nil {
		return rep, NotFound("no shift")
	}
	return rep, nil
}

func (s *ReceiptService) CloseShift(ctx context.Context, shiftID int64, actualCash *float64, userID int64) (map[string]interface{}, error) {
	rep, err := s.Shifts.Report(ctx, s.Store.PG, shiftID)
	if err != nil {
		return nil, NotFound("no shift")
	}
	actual := rep.ExpectedCash
	if actualCash != nil {
		actual = *actualCash
	}
	z := map[string]interface{}{
		"sale_count": rep.SaleCount, "return_count": rep.ReturnCount,
		"cash_sales": rep.CashSales, "card_sales": rep.CardSales,
		"cash_returns": rep.CashReturns, "card_returns": rep.CardReturns,
		"start_cash": rep.StartCash, "expected_cash": rep.ExpectedCash,
		"actual_cash": actual, "discrepancy": model.Round2(actual - rep.ExpectedCash),
	}
	if err := s.Shifts.Close(ctx, s.Store.PG, shiftID, userID, actual, z); err != nil {
		return nil, Conflict("close failed (already closed?)")
	}
	return z, nil
}

// --- Items ---

type SellItemInput struct {
	ProductID    *int64   `json:"product_id"`
	Code         string   `json:"code"`
	Quantity     float64  `json:"quantity"`
	Price        *float64 `json:"price"`
	Discount     float64  `json:"discount"`
	ItemAttr     string   `json:"ffd_item_attribute"`
	PayMethod    string   `json:"ffd_payment_method"`
	MarkingCodes []string `json:"marking_codes"`
	BookingID    *int64   `json:"booking_id"`
}

// ResolveItems проверяет товары, подставляет цену (явная → розница → базовая).
func (s *ReceiptService) ResolveItems(ctx context.Context, db repository.DBTX, org int64, in []SellItemInput) ([]model.ReceiptLine, error) {
	var out []model.ReceiptLine
	for _, it := range in {
		if it.Quantity <= 0 {
			return nil, BadRequest("quantity must be > 0")
		}
		var p repository.SaleProduct
		var err error
		if it.ProductID != nil {
			p, err = s.Products.ForSaleByID(ctx, db, *it.ProductID)
		} else if it.Code != "" {
			p, err = s.Products.ForSaleByCode(ctx, db, it.Code)
		} else {
			return nil, BadRequest("product_id or code required")
		}
		if err != nil {
			return nil, NotFound("product not found")
		}
		if !p.Active {
			return nil, BadRequest("product inactive")
		}
		price := 0.0
		hasPrice := false
		if it.Price != nil {
			price, hasPrice = *it.Price, true
		} else if rp := s.Products.RetailPrice(ctx, db, p.ID, org); rp != nil {
			price, hasPrice = *rp, true
		} else if bp := s.Products.BasePrice(ctx, db, p.ID); bp != nil {
			price, hasPrice = *bp, true
		}
		if !hasPrice || price < 0 {
			return nil, BadRequest("price missing/invalid")
		}
		attr := it.ItemAttr
		if attr == "" {
			attr = "GOOD"
			if p.Marked {
				attr = "MARKED"
			}
		}
		method := it.PayMethod
		if method == "" {
			method = "FULL"
		}
		var codeIDs []int64
		if p.Marked {
			if len(it.MarkingCodes) != int(it.Quantity) {
				return nil, BadRequest("marked product needs one code per unit")
			}
			codeIDs, err = s.Marking.LockCodes(ctx, db, org, p.ID, it.MarkingCodes)
			if err != nil {
				return nil, BadRequest(err.Error())
			}
		}
		if it.BookingID != nil {
			if err := s.Receipts.CheckBookingLink(ctx, db, *it.BookingID, p.ID, org); err != nil {
				return nil, BadRequest(err.Error())
			}
		}
		out = append(out, model.ReceiptLine{
			ProductID: p.ID, Name: p.Name, SKU: p.SKU, Qty: it.Quantity,
			Price: model.Round2(price), VAT: p.VAT, Discount: model.Round2(it.Discount),
			Marked: p.Marked, Attr: attr, Method: method, CodeIDs: codeIDs, BookingID: it.BookingID,
		})
	}
	return out, nil
}

func mapStockErr(err error) *Error {
	if err == repository.ErrNoStock {
		return Conflict("no stock for product")
	}
	if err == repository.ErrInsufficientStock {
		return Conflict("insufficient stock")
	}
	return Conflict("stock operation failed")
}

// --- Sell ---

type SellInput struct {
	CashRegisterID int64           `json:"cash_register_id"`
	Items          []SellItemInput `json:"items"`
	PaymentType    string          `json:"payment_type"`
	PaymentCash    float64         `json:"payment_cash"`
	PaymentCard    float64         `json:"payment_card"`
}

func (s *ReceiptService) Sell(ctx context.Context, in SellInput, cashierID int64, ip, ua string) (int64, string, error) {
	if in.CashRegisterID == 0 || len(in.Items) == 0 {
		return 0, "", BadRequest("cash_register_id/items required")
	}
	if in.PaymentType == "" {
		in.PaymentType = "CASH"
	}
	var rid int64
	var number string
	var saleTotal float64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		org, warehouse, err := s.Registers.OrgWarehouse(ctx, tx, in.CashRegisterID)
		if err != nil {
			return NotFound("no register")
		}
		shift, _, err := s.Shifts.GetOpen(ctx, tx, in.CashRegisterID)
		if err != nil {
			return Conflict("shift not open")
		}
		lines, err := s.ResolveItems(ctx, tx, org, in.Items)
		if err != nil {
			return err
		}
		if warehouse != nil {
			if err := s.Balances.Deduct(ctx, tx, *warehouse, model.LineQty(lines)); err != nil {
				return mapStockErr(err)
			}
		}
		total, vatSum, _ := model.LineTotals(lines)
		if model.Round2(in.PaymentCash+in.PaymentCard) < total {
			return BadRequest("paid < total")
		}
		rid, number, err = s.Receipts.Insert(ctx, tx, org, in.CashRegisterID, shift, cashierID,
			"SALE", lines, total, vatSum, in.PaymentType, in.PaymentCash, in.PaymentCard, nil, "")
		if err != nil {
			return Conflict("sell failed")
		}
		saleTotal = total
		s.Notify.EnqueueTx(ctx, tx, org, "RECEIPT_SOLD", []string{"WEB"},
			s.Notify.RecipientOf(ctx, tx, cashierID), "", "",
			map[string]interface{}{"receipt_number": number, "total_amount": saleTotal,
				"payment_type": in.PaymentType, "fiscal_doc": ""}, "receipt", &rid, 5)
		if warehouse != nil {
			s.Notify.CheckLowStock(ctx, tx, org, *warehouse)
		}
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, "", se
		}
		return 0, "", Conflict("sell failed")
	}
	s.Audit.Log(ctx, s.Store.PG, &cashierID, "receipt.sell", "Пробитие чека "+number, "receipt", &rid, in, ip, ua, true, "")
	return rid, number, nil
}

// --- Return ---

type ReturnItemInput struct {
	ProductID    int64    `json:"product_id"`
	Quantity     float64  `json:"quantity"`
	MarkingCodes []string `json:"marking_codes"`
}

type ReturnInput struct {
	BaseReceiptID int64             `json:"base_receipt_id"`
	Items         []ReturnItemInput `json:"items"`
	PaymentType   string            `json:"payment_type"`
	PaymentCash   float64           `json:"payment_cash"`
	PaymentCard   float64           `json:"payment_card"`
}

func (s *ReceiptService) Return(ctx context.Context, in ReturnInput, cashierID int64, ip, ua string) (int64, string, error) {
	if in.BaseReceiptID == 0 || len(in.Items) == 0 {
		return 0, "", BadRequest("base_receipt_id/items required")
	}
	if in.PaymentType == "" {
		in.PaymentType = "CASH"
	}
	var rid int64
	var number string
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		org, reg, shift, rtype, err := s.Receipts.BaseForReturn(ctx, tx, in.BaseReceiptID)
		if err != nil {
			return NotFound("base receipt not found")
		}
		if rtype != "SALE" {
			return BadRequest("only SALE can be returned")
		}
		if err := s.Receipts.ShiftOpen(ctx, tx, shift); err != nil {
			return Conflict("base shift closed")
		}
		var lines []model.ReceiptLine
		var retCodes []int64
		for _, it := range in.Items {
			if it.Quantity <= 0 {
				return BadRequest("quantity must be > 0")
			}
			base, err := s.Receipts.BaseItem(ctx, tx, in.BaseReceiptID, it.ProductID)
			if err != nil {
				return BadRequest("item not in base receipt")
			}
			already := s.Receipts.ReturnedQty(ctx, tx, in.BaseReceiptID, it.ProductID)
			if it.Quantity > base.Sold-already {
				return BadRequest("return qty exceeds sold")
			}
			if base.Marked {
				if len(it.MarkingCodes) != int(it.Quantity) {
					return BadRequest("marked return needs one code per unit")
				}
				for _, raw := range it.MarkingCodes {
					code := strings.TrimSpace(raw)
					cid, err := s.Receipts.SoldCodeInReceipt(ctx, tx, code, in.BaseReceiptID)
					if err != nil {
						return BadRequest(err.Error())
					}
					retCodes = append(retCodes, cid)
				}
			}
			lines = append(lines, model.ReceiptLine{
				ProductID: it.ProductID, Name: base.Name, SKU: base.SKU, Qty: it.Quantity,
				Price: base.Price, VAT: base.VAT, Marked: base.Marked, Attr: "GOOD", Method: "FULL",
			})
		}
		total, vatSum, _ := model.LineTotals(lines)
		if model.Round2(in.PaymentCash+in.PaymentCard) < total {
			return BadRequest("paid < total")
		}
		rid, number, err = s.Receipts.Insert(ctx, tx, org, reg, shift, cashierID,
			"RETURN", lines, total, vatSum, in.PaymentType, in.PaymentCash, in.PaymentCard, &in.BaseReceiptID, "")
		if err != nil {
			return Conflict("return failed")
		}
		if len(retCodes) > 0 {
			if err := s.Marking.Return(ctx, tx, org, rid, retCodes); err != nil {
				return Conflict("return failed")
			}
		}
		_, warehouse, err := s.Registers.OrgWarehouse(ctx, tx, reg)
		if err == nil && warehouse != nil {
			if err := s.Balances.Add(ctx, tx, *warehouse, model.LineQty(lines)); err != nil {
				return mapStockErr(err)
			}
		}
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, "", se
		}
		return 0, "", Conflict("return failed")
	}
	s.Audit.Log(ctx, s.Store.PG, &cashierID, "receipt.return", "Возврат по чеку", "receipt", &rid, in, ip, ua, true, "")
	return rid, number, nil
}

// --- Correction ---

type CorrectionInput struct {
	CashRegisterID int64           `json:"cash_register_id"`
	Items          []SellItemInput `json:"items"`
	PaymentType    string          `json:"payment_type"`
	PaymentCash    float64         `json:"payment_cash"`
	PaymentCard    float64         `json:"payment_card"`
	Reason         string          `json:"reason"`
}

func (s *ReceiptService) Correction(ctx context.Context, in CorrectionInput, cashierID int64) (int64, string, error) {
	if in.CashRegisterID == 0 || len(in.Items) == 0 || in.Reason == "" {
		return 0, "", BadRequest("cash_register_id/items/reason required")
	}
	if in.PaymentType == "" {
		in.PaymentType = "CASH"
	}
	var rid int64
	var number string
	var saleTotal float64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		org, warehouse, err := s.Registers.OrgWarehouse(ctx, tx, in.CashRegisterID)
		if err != nil {
			return NotFound("no register")
		}
		shift, _, err := s.Shifts.GetOpen(ctx, tx, in.CashRegisterID)
		if err != nil {
			return Conflict("shift not open")
		}
		lines, err := s.ResolveItems(ctx, tx, org, in.Items)
		if err != nil {
			return err
		}
		if warehouse != nil {
			if err := s.Balances.Deduct(ctx, tx, *warehouse, model.LineQty(lines)); err != nil {
				return mapStockErr(err)
			}
		}
		total, vatSum, _ := model.LineTotals(lines)
		if model.Round2(in.PaymentCash+in.PaymentCard) < total {
			return BadRequest("paid < total")
		}
		rid, number, err = s.Receipts.Insert(ctx, tx, org, in.CashRegisterID, shift, cashierID,
			"CORRECTION", lines, total, vatSum, in.PaymentType, in.PaymentCash, in.PaymentCard, nil, in.Reason)
		if err != nil {
			return Conflict("correction failed")
		}
		saleTotal = total
		s.Notify.EnqueueTx(ctx, tx, org, "RECEIPT_SOLD", []string{"WEB"},
			s.Notify.RecipientOf(ctx, tx, cashierID), "", "",
			map[string]interface{}{"receipt_number": number, "total_amount": saleTotal,
				"payment_type": in.PaymentType, "fiscal_doc": ""}, "receipt", &rid, 5)
		if warehouse != nil {
			s.Notify.CheckLowStock(ctx, tx, org, *warehouse)
		}
		return nil
	})
	if err != nil {
		if se, ok := err.(*Error); ok {
			return 0, "", se
		}
		return 0, "", Conflict("correction failed")
	}
	return rid, number, nil
}

// --- Lists / OFD settings ---

func (s *ReceiptService) ListReceipts(ctx context.Context, shiftID int64, limit int) []model.Receipt {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.Receipts.List(ctx, s.Store.PG, shiftID, limit)
}

func (s *ReceiptService) GetOfdSettings(ctx context.Context, orgID int64) (repository.OfdSettings, error) {
	if orgID == 0 {
		return repository.OfdSettings{}, BadRequest("org_id required")
	}
	st, err := s.Ofd.GetSettings(ctx, s.Store.PG, orgID)
	if err != nil {
		return st, NotFound("no settings")
	}
	return st, nil
}

func (s *ReceiptService) PatchOfdSettings(ctx context.Context, orgID int64, raw map[string]interface{}) error {
	if orgID == 0 {
		return BadRequest("org_id required")
	}
	var failFirst *int
	if v, ok := raw["fail_first_attempts"].(float64); ok {
		f := int(v)
		failFirst = &f
	}
	var auto *bool
	if v, ok := raw["auto_send_enabled"].(bool); ok {
		auto = &v
	}
	s.Ofd.PatchSettings(ctx, s.Store.PG, orgID, failFirst, auto)
	return nil
}
