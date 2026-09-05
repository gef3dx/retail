package service

import (
	"context"

	"retail-backend/internal/provider"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// IntegrationService — настройки провайдеров организации.
type IntegrationService struct {
	Store *store.Store
	Regs  repository.IntegrationRepo
	Reg   *provider.Registry
}

func (s *IntegrationService) Statuses(ctx context.Context, orgID int64) ([]provider.ProviderStatus, error) {
	if orgID == 0 {
		return nil, BadRequest("org_id required")
	}
	return s.Regs.Statuses(ctx, s.Store.PG, s.Reg, orgID), nil
}

type SaveIntegrationInput struct {
	Credentials map[string]string `json:"credentials"`
	Enabled     *bool             `json:"enabled"`
}

func (s *IntegrationService) Save(ctx context.Context, orgID int64, code string, in SaveIntegrationInput) error {
	if orgID == 0 || code == "" {
		return BadRequest("org_id/code required")
	}
	if s.Reg.ByCode(code) == nil {
		return NotFound("unknown provider")
	}
	if err := s.Regs.Upsert(ctx, s.Store.PG, s.Reg, orgID, code, in.Credentials, in.Enabled); err != nil {
		return BadRequest("save failed")
	}
	return nil
}

func (s *IntegrationService) Clear(ctx context.Context, orgID int64, code string) error {
	if orgID == 0 || code == "" {
		return BadRequest("org_id/code required")
	}
	s.Regs.Clear(ctx, s.Store.PG, orgID, code)
	return nil
}

// Test проверяет соединение: эмулятор — всегда ok, реал — наличие ключей
// (живые проверки — на этапах 11-15).
func (s *IntegrationService) Test(ctx context.Context, orgID int64, code string) (bool, string, error) {
	p := s.Reg.ByCode(code)
	if p == nil {
		return false, "", NotFound("unknown provider")
	}
	creds, _, _ := s.Regs.Get(ctx, s.Store.PG, orgID, code)
	ok, msg := p.Test(creds)
	return ok, msg, nil
}

// RequireActive возвращает ошибку 409, если провайдер домена не активен.
// Используется этапами 11-15 для блокировки функций без ключей.
func (s *IntegrationService) RequireActive(ctx context.Context, orgID int64, kind provider.Kind) (string, error) {
	statuses := s.Regs.Statuses(ctx, s.Store.PG, s.Reg, orgID)
	if code := s.Reg.ActiveFor(kind, statuses); code != "" {
		return code, nil
	}
	return "", Conflict("no active " + string(kind) + " provider: configure integration keys")
}
