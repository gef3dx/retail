package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// TaxService — книги, закрытия периодов, декларации, экспорт, сводки.
type TaxService struct {
	Store *store.Store
	Tax   repository.TaxRepo
}

func (s *TaxService) SalesBook(ctx context.Context, orgID, year, quarter int) ([]model.BookEntry, error) {
	out, err := s.Tax.LiveSalesBook(ctx, s.Store.PG, orgID, year, quarter)
	if err != nil {
		return nil, BadRequest("bad period (year/quarter)")
	}
	return out, nil
}

func (s *TaxService) PurchaseBook(ctx context.Context, orgID, year, quarter int) ([]model.BookEntry, error) {
	out, err := s.Tax.LivePurchaseBook(ctx, s.Store.PG, orgID, year, quarter)
	if err != nil {
		return nil, BadRequest("bad period (year/quarter)")
	}
	return out, nil
}

func (s *TaxService) ClosePeriod(ctx context.Context, orgID, year, quarter int, declType string) (model.Declaration, error) {
	if declType == "" {
		declType = "NDS"
	}
	if declType != "NDS" && declType != "USN" && declType != "PROFIT" {
		return model.Declaration{}, BadRequest("decl_type NDS/USN/PROFIT")
	}
	var d model.Declaration
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		d, err = s.Tax.ClosePeriod(ctx, tx, orgID, year, quarter, declType)
		return err
	})
	if err != nil {
		return d, BadRequest("bad period (year/quarter)")
	}
	return d, nil
}

func (s *TaxService) Declarations(ctx context.Context, orgID int64) []model.Declaration {
	return s.Tax.Declarations(ctx, s.Store.PG, orgID)
}

func (s *TaxService) Submit(ctx context.Context, orgID, id int64) error {
	if !s.Tax.SubmitDeclaration(ctx, s.Store.PG, orgID, id) {
		return NotFound("no declaration")
	}
	return nil
}

// ExportCSV выгружает книгу периода в CSV (открывается в Excel).
func (s *TaxService) ExportCSV(ctx context.Context, orgID, year, quarter int, book string) (string, string, error) {
	var entries []model.BookEntry
	var err error
	if book == "purchase" {
		entries, err = s.PurchaseBook(ctx, orgID, year, quarter)
	} else {
		entries, err = s.SalesBook(ctx, orgID, year, quarter)
	}
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	b.WriteString("N;DocType;DocNumber;DocDate;INN;Amount;VAT;Total\n")
	q := func(v string) string { return strings.ReplaceAll(v, ";", ",") }
	num := func(f float64) string { return strings.ReplaceAll(fmt.Sprintf("%.2f", f), ".", ",") }
	safe := func(p *string) string {
		if p == nil {
			return ""
		}
		return q(*p)
	}
	for _, e := range entries {
		fmt.Fprintf(&b, "%d;%s;%s;%s;%s;%s;%s;%s\n",
			e.Number, q(e.DocType), safe(e.DocNum), safe(e.DocDate), safe(e.Counter),
			num(e.Amount), num(e.VAT), num(e.Total))
	}
	name := fmt.Sprintf("%s_%d_q%d.csv", book, year, quarter)
	return name, "\xef\xbb\xbf" + b.String(), nil // BOM для Excel
}

func (s *TaxService) Summary(ctx context.Context, orgID int64) model.TaxSummary {
	return s.Tax.Summary(ctx, s.Store.PG, orgID)
}
