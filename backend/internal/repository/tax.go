package repository

import (
	"context"
	"fmt"

	"retail-backend/internal/model"
)

// TaxRepo — книги, закрытия периодов, декларации, сводки.
type TaxRepo struct{}

// quarterRange возвращает границы квартала.
func quarterRange(year, quarter int) (string, string, error) {
	if quarter < 1 || quarter > 4 || year < 2000 || year > 2100 {
		return "", "", fmt.Errorf("bad period")
	}
	starts := map[int]string{1: "01-01", 2: "04-01", 3: "07-01", 4: "10-01"}
	ends := map[int]string{1: "04-01", 2: "07-01", 3: "10-01", 4: "01-01"}
	endYear := year
	if quarter == 4 {
		endYear++
	}
	return fmt.Sprintf("%d-%s", year, starts[quarter]),
		fmt.Sprintf("%d-%s", endYear, ends[quarter]), nil
}

// LiveSalesBook вычисляет книгу продаж из чеков периода (включая возвраты минусом).
func (TaxRepo) LiveSalesBook(ctx context.Context, db DBTX, orgID, year, quarter int) ([]model.BookEntry, error) {
	from, to, err := quarterRange(year, quarter)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		SELECT r.receipt_number, r.created_at::date::text, r.total_amount, r.total_vat, r.receipt_type
		FROM sales_receipt r
		WHERE r.organization_id=$1 AND r.created_at >= $2::date AND r.created_at < $3::date
		ORDER BY r.id`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.BookEntry
	n := 0
	for rows.Next() {
		var num, dt, rtype string
		var total, vat float64
		_ = rows.Scan(&num, &dt, &total, &vat, &rtype)
		sign := 1.0
		dt2 := "Продажа"
		if rtype == "RETURN" {
			sign = -1.0
			dt2 = "Возврат"
		} else if rtype == "CORRECTION" {
			dt2 = "Коррекция"
		}
		n++
		out = append(out, model.BookEntry{
			Number: n, DocType: dt2, DocNum: &num, DocDate: &dt,
			Amount: total * sign, VAT: vat * sign, Total: total * sign,
		})
	}
	if out == nil {
		out = []model.BookEntry{}
	}
	return out, rows.Err()
}

// LivePurchaseBook вычисляет книгу покупок из проведенных поступлений.
func (TaxRepo) LivePurchaseBook(ctx context.Context, db DBTX, orgID, year, quarter int) ([]model.BookEntry, error) {
	from, to, err := quarterRange(year, quarter)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		SELECT d.document_number, d.document_date::text, c.inn,
		       d.total_amount, d.total_vat
		FROM receipt_document d JOIN counterparty c ON c.id=d.supplier_id
		WHERE d.organization_id=$1 AND d.is_posted AND d.document_date >= $2::date AND d.document_date < $3::date
		ORDER BY d.id`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.BookEntry
	n := 0
	for rows.Next() {
		var e model.BookEntry
		var num, dt, inn string
		var total, vat float64
		_ = rows.Scan(&num, &dt, &inn, &total, &vat)
		n++
		e = model.BookEntry{Number: n, DocType: "Поступление", DocNum: &num, DocDate: &dt,
			Counter: &inn, Amount: total - vat, VAT: vat, Total: total}
		out = append(out, e)
	}
	if out == nil {
		out = []model.BookEntry{}
	}
	return out, rows.Err()
}

// ClosePeriod перезаписывает снапшот книг и декларацию за квартал (идемпотентно).
func (TaxRepo) ClosePeriod(ctx context.Context, db DBTX, orgID, year, quarter int, declType string) (model.Declaration, error) {
	var d model.Declaration
	sales, err := (TaxRepo{}).LiveSalesBook(ctx, db, orgID, year, quarter)
	if err != nil {
		return d, err
	}
	purch, err := (TaxRepo{}).LivePurchaseBook(ctx, db, orgID, year, quarter)
	if err != nil {
		return d, err
	}
	_, _ = db.Exec(ctx, `DELETE FROM sales_book WHERE organization_id=$1 AND year=$2 AND quarter=$3`, orgID, year, quarter)
	_, _ = db.Exec(ctx, `DELETE FROM purchase_book WHERE organization_id=$1 AND year=$2 AND quarter=$3`, orgID, year, quarter)
	var ts, tvOut, tp, tvIn float64
	for _, e := range sales {
		ts += e.Total
		tvOut += e.VAT
		_, _ = db.Exec(ctx, `
			INSERT INTO sales_book(organization_id, year, quarter, entry_number, document_type,
				document_number, document_date, sales_amount, vat_amount, total_amount)
			VALUES($1,$2,$3,$4,$5,$6,$7::date,$8,$9,$10)`,
			orgID, year, quarter, e.Number, e.DocType, e.DocNum, e.DocDate, e.Total-e.VAT, e.VAT, e.Total)
	}
	for _, e := range purch {
		tp += e.Total
		tvIn += e.VAT
		_, _ = db.Exec(ctx, `
			INSERT INTO purchase_book(organization_id, year, quarter, entry_number, document_type,
				document_number, document_date, supplier_inn, purchase_amount, vat_amount, total_amount)
			VALUES($1,$2,$3,$4,$5,$6,$7::date,$8,$9,$10,$11)`,
			orgID, year, quarter, e.Number, e.DocType, e.DocNum, e.DocDate,
			nullableStr(e.Counter), e.Amount, e.VAT, e.Total)
	}
	vatDue := tvOut - tvIn
	err = db.QueryRow(ctx, `
		INSERT INTO tax_declaration(organization_id, year, quarter, decl_type,
			total_sales, total_vat_out, total_purchases, total_vat_in, vat_due)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (organization_id, year, quarter, decl_type)
		DO UPDATE SET total_sales=$5, total_vat_out=$6, total_purchases=$7,
			total_vat_in=$8, vat_due=$9, status='DRAFT'
		RETURNING id, total_sales, total_vat_out, total_purchases, total_vat_in, vat_due, status`,
		orgID, year, quarter, declType, ts, tvOut, tp, tvIn, vatDue).
		Scan(&d.ID, &d.TotalSales, &d.VATOut, &d.TotalPurch, &d.VATIn, &d.VATDue, &d.Status)
	if err != nil {
		return d, err
	}
	d.Year, d.Quarter, d.Type = year, quarter, declType
	return d, nil
}

func nullableStr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func (TaxRepo) Declarations(ctx context.Context, db DBTX, orgID int64) []model.Declaration {
	rows, err := db.Query(ctx, `
		SELECT id, year, quarter, decl_type, total_sales, total_vat_out,
		       total_purchases, total_vat_in, vat_due, status
		FROM tax_declaration WHERE organization_id=$1 ORDER BY year DESC, quarter DESC`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Declaration
	for rows.Next() {
		var d model.Declaration
		_ = rows.Scan(&d.ID, &d.Year, &d.Quarter, &d.Type, &d.TotalSales, &d.VATOut, &d.TotalPurch, &d.VATIn, &d.VATDue, &d.Status)
		out = append(out, d)
	}
	if out == nil {
		out = []model.Declaration{}
	}
	return out
}

func (TaxRepo) SubmitDeclaration(ctx context.Context, db DBTX, orgID, id int64) bool {
	res, err := db.Exec(ctx, `UPDATE tax_declaration SET status='SUBMITTED' WHERE id=$1 AND organization_id=$2`, id, orgID)
	if err != nil || res.RowsAffected() == 0 {
		return false
	}
	return true
}

func (TaxRepo) Summary(ctx context.Context, db DBTX, orgID int64) model.TaxSummary {
	var s model.TaxSummary
	_ = db.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_amount),0), COUNT(*), COALESCE(SUM(total_vat),0)
		FROM sales_receipt
		WHERE organization_id=$1 AND receipt_type='SALE' AND created_at > NOW() - INTERVAL '30 days'`, orgID).
		Scan(&s.Revenue30, &s.Receipts30, &s.VATOut30)
	rows, err := db.Query(ctx, `
		SELECT created_at::date::text, SUM(total_amount) FROM sales_receipt
		WHERE organization_id=$1 AND receipt_type='SALE' AND created_at > NOW() - INTERVAL '14 days'
		GROUP BY 1 ORDER BY 1`, orgID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dr model.DayRevenue
			_ = rows.Scan(&dr.Day, &dr.Total)
			s.ByDay = append(s.ByDay, dr)
		}
	}
	trows, err := db.Query(ctx, `
		SELECT p.sku, p.name, SUM(i.quantity), SUM(i.total_amount)
		FROM sales_receipt_item i
		JOIN sales_receipt r ON r.id=i.receipt_id
		JOIN catalog_product p ON p.id=i.product_id
		WHERE r.organization_id=$1 AND r.receipt_type='SALE' AND r.created_at > NOW() - INTERVAL '30 days'
		GROUP BY 1, 2 ORDER BY 4 DESC LIMIT 5`, orgID)
	if err == nil {
		defer trows.Close()
		for trows.Next() {
			var tp model.TopProduct
			_ = trows.Scan(&tp.SKU, &tp.Name, &tp.Qty, &tp.Total)
			s.Top = append(s.Top, tp)
		}
	}
	if s.ByDay == nil {
		s.ByDay = []model.DayRevenue{}
	}
	if s.Top == nil {
		s.Top = []model.TopProduct{}
	}
	return s
}
