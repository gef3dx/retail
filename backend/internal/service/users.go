package service

import (
	"context"

	"retail-backend/internal/auth"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// UserService — управление пользователями. Орг-сервис — ниже.
type UserService struct {
	Store *store.Store
	Users repository.UserRepo
	Audit repository.AuditRepo
}

type CreateUserInput struct {
	Username       string `json:"username"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	OrganizationID *int64 `json:"organization_id"`
	Role           string `json:"role"`
}

func (s *UserService) List(ctx context.Context) []model.User {
	return s.Users.List(ctx, s.Store.PG)
}

func (s *UserService) Create(ctx context.Context, in CreateUserInput, actorID int64, ip, ua string) (int64, error) {
	if in.Username == "" || in.Email == "" || len(in.Password) < 6 {
		return 0, BadRequest("username/email/password>=6 required")
	}
	hash, _ := auth.HashPassword(in.Password)
	id, err := s.Users.Create(ctx, s.Store.PG, in.Username, in.Email, hash, in.FirstName, in.LastName)
	if err != nil {
		return 0, Conflict("duplicate username/email")
	}
	if in.OrganizationID != nil {
		s.Users.LinkOrg(ctx, s.Store.PG, id, *in.OrganizationID)
		role := in.Role
		if role == "" {
			role = "VIEWER"
		}
		_, _ = s.Users.GrantRole(ctx, s.Store.PG, id, role, in.OrganizationID)
	}
	s.Audit.Log(ctx, s.Store.PG, &actorID, "user.create", "Создание пользователя", "user", &id, in, ip, ua, true, "")
	return id, nil
}

func (s *UserService) AssignRole(ctx context.Context, id int64, role string, orgID *int64, actorID int64, ip, ua string) error {
	if role == "" {
		return BadRequest("role required")
	}
	ok, err := s.Users.GrantRole(ctx, s.Store.PG, id, role, orgID)
	if err != nil {
		return BadRequest("assign failed")
	}
	body := map[string]interface{}{"role": role, "organization_id": orgID}
	s.Audit.Log(ctx, s.Store.PG, &actorID, "user.role", "Назначение роли "+role, "user", &id, body, ip, ua, ok, "")
	return nil
}

// OrgService — организации и справочник ролей.
type OrgService struct {
	Store *store.Store
	Orgs  repository.OrgRepo
	Audit repository.AuditRepo
}

func (s *OrgService) List(ctx context.Context) []model.Organization {
	return s.Orgs.List(ctx, s.Store.PG)
}

func (s *OrgService) Create(ctx context.Context, inn, kpp, fullName, shortName string, actorID int64, ip, ua string) (int64, error) {
	if inn == "" || kpp == "" || fullName == "" {
		return 0, BadRequest("inn/kpp/full_name required")
	}
	id, err := s.Orgs.Create(ctx, s.Store.PG, inn, kpp, fullName, shortName)
	if err != nil {
		return 0, Conflict("duplicate inn")
	}
	s.Orgs.EnsureDefaults(ctx, s.Store.PG, id)
	s.Audit.Log(ctx, s.Store.PG, &actorID, "org.create", "Создание организации", "organization", &id,
		map[string]string{"inn": inn, "name": fullName}, ip, ua, true, "")
	return id, nil
}

func (s *OrgService) Roles(ctx context.Context) []model.Role {
	return s.Orgs.ListRoles(ctx, s.Store.PG)
}

func (s *OrgService) AuditLog(ctx context.Context) []repository.Entry {
	return repository.AuditRepo{}.List(ctx, s.Store.PG)
}
