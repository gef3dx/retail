package service

import (
	"context"
	"strings"

	"retail-backend/internal/egais"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// EgaisService — УТМ: диагностика и отправка XML-документов.
type EgaisService struct {
	Store *store.Store
	Docs  repository.EgaisRepo
	Int   repository.IntegrationRepo
}

func (s *EgaisService) utmURL(ctx context.Context, orgID int64) (string, error) {
	creds, enabled, found := s.Int.Get(ctx, s.Store.PG, orgID, "EGAIS_UTM")
	if !found || !enabled || strings.TrimSpace(creds["utm_url"]) == "" {
		return "", Conflict("UTM not configured: save utm_url in integrations")
	}
	return creds["utm_url"], nil
}

// Status опрашивает самодиагностику УТМ.
func (s *EgaisService) Status(ctx context.Context, orgID int64) (model.UtmStatus, error) {
	url, err := s.utmURL(ctx, orgID)
	if err != nil {
		return model.UtmStatus{Reachable: false, Error: err.Error()}, err
	}
	ver, err := (egais.UTM{}).Check(ctx, url)
	if err != nil {
		return model.UtmStatus{Reachable: false, URL: url, Error: err.Error()}, Conflict("utm unreachable: " + err.Error())
	}
	return model.UtmStatus{Reachable: true, Version: ver, URL: url}, nil
}

type CreateEgaisDocInput struct {
	OrgID int64  `json:"org_id"`
	Type  string `json:"doc_type"`
	XML   string `json:"xml"`
}

// CreateDoc сохраняет документ и сразу отправляет в УТМ.
func (s *EgaisService) CreateDoc(ctx context.Context, in CreateEgaisDocInput, userID int64) (model.EgaisDoc, error) {
	var out model.EgaisDoc
	if in.OrgID == 0 || in.Type == "" || !strings.Contains(in.XML, "<") {
		return out, BadRequest("org_id/doc_type/xml required")
	}
	id, err := s.Docs.Create(ctx, s.Store.PG, in.OrgID, in.Type, in.XML)
	if err != nil {
		return out, BadRequest("create failed")
	}
	url, err := s.utmURL(ctx, in.OrgID)
	if err != nil {
		s.Docs.SetResult(ctx, s.Store.PG, id, "FAILED", "", "UTM not configured")
		return s.get(ctx, in.OrgID, id)
	}
	reply, err := (egais.UTM{}).SendDoc(ctx, url, in.XML)
	if err != nil {
		s.Docs.SetResult(ctx, s.Store.PG, id, "FAILED", "", err.Error())
		_ = userID
		return s.get(ctx, in.OrgID, id)
	}
	s.Docs.SetResult(ctx, s.Store.PG, id, "ACCEPTED", reply, "")
	return s.get(ctx, in.OrgID, id)
}

func (s *EgaisService) get(ctx context.Context, orgID, id int64) (model.EgaisDoc, error) {
	for _, d := range s.Docs.List(ctx, s.Store.PG, orgID) {
		if d.ID == id {
			return d, nil
		}
	}
	return model.EgaisDoc{}, NotFound("no document")
}

func (s *EgaisService) List(ctx context.Context, orgID int64) []model.EgaisDoc {
	return s.Docs.List(ctx, s.Store.PG, orgID)
}
