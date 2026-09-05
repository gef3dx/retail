package service

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// MarkingService — пул кодов, проверка, списание, очередь ГИС МТ.
type MarkingService struct {
	Store *store.Store
	Codes repository.CodesRepo
	Audit repository.AuditRepo
}

type RegisterCodesInput struct {
	OrgID     int64    `json:"org_id"`
	ProductID int64    `json:"product_id"`
	Codes     []string `json:"codes"`
	Batch     string   `json:"batch_number"`
}

type RejectedCode struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type RegisterResult struct {
	Registered int            `json:"registered"`
	Duplicates int            `json:"duplicates"`
	Rejected   []RejectedCode `json:"rejected"`
	Product    string         `json:"product"`
}

func (s *MarkingService) Register(ctx context.Context, in RegisterCodesInput, actorID int64, ip, ua string) (RegisterResult, error) {
	var res RegisterResult
	if in.OrgID == 0 || in.ProductID == 0 || len(in.Codes) == 0 {
		return res, BadRequest("org_id/product_id/codes[] required")
	}
	gtin, pname, marked, err := s.Codes.ProductForRegister(ctx, s.Store.PG, in.ProductID)
	if err != nil {
		return res, NotFound("no product")
	}
	if !marked {
		return res, BadRequest("product is not marked")
	}
	var clean []string
	seen := map[string]bool{}
	for _, raw := range in.Codes {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		if seen[code] {
			res.Rejected = append(res.Rejected, RejectedCode{code, "duplicate in request"})
			continue
		}
		seen[code] = true
		if gtin != "" && !strings.Contains(code, gtin) {
			res.Rejected = append(res.Rejected, RejectedCode{code, "gtin mismatch"})
			continue
		}
		clean = append(clean, code)
	}
	err = s.Store.Tx(ctx, func(tx pgx.Tx) error {
		var batchID *int64
		if in.Batch != "" {
			bid, err := s.Codes.CreateBatch(ctx, tx, in.OrgID, in.ProductID, in.Batch, len(clean))
			if err != nil {
				return err
			}
			batchID = &bid
		}
		for _, code := range clean {
			if _, ok := s.Codes.InsertCode(ctx, tx, in.OrgID, in.ProductID, code, gtin, batchID); ok {
				res.Registered++
			} else {
				res.Duplicates++
			}
		}
		return nil
	})
	if err != nil {
		return res, Conflict("register failed (duplicate batch?)")
	}
	res.Product = pname
	s.Audit.Log(ctx, s.Store.PG, &actorID, "marking.register",
		"Регистрация кодов маркировки", "product", &in.ProductID,
		map[string]interface{}{"registered": res.Registered}, ip, ua, true, "")
	return res, nil
}

func (s *MarkingService) List(ctx context.Context, orgID int64, productID, status, q string, limit int) []model.MarkingCode {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.Codes.List(ctx, s.Store.PG, repository.CodeFilter{
		OrgID: orgID, ProductID: productID, Status: status, Q: strings.TrimSpace(q), Limit: limit,
	})
}

func (s *MarkingService) Check(ctx context.Context, code string) (model.MarkingCheck, error) {
	c, err := s.Codes.Check(ctx, s.Store.PG, strings.TrimSpace(code))
	if err != nil {
		return c, NotFound("code unknown")
	}
	return c, nil
}

func (s *MarkingService) WriteOff(ctx context.Context, code, reason, username string) error {
	if strings.TrimSpace(code) == "" {
		return BadRequest("code required")
	}
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		return s.Codes.WriteOff(ctx, tx, strings.TrimSpace(code), reason, username)
	})
	if err != nil {
		if msg := strings.TrimPrefix(err.Error(), "conflict: "); msg != err.Error() {
			return Conflict(msg)
		}
		return NotFound("code unknown")
	}
	return nil
}

func (s *MarkingService) Queue(ctx context.Context, orgID int64, status string) []model.GismtQueueItem {
	return s.Codes.Queue(ctx, s.Store.PG, orgID, status)
}

func (s *MarkingService) Log(ctx context.Context, orgID int64, itype string) []model.IntegrationLogEntry {
	return s.Codes.Log(ctx, s.Store.PG, orgID, itype)
}
