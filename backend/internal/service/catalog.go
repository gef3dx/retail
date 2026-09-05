package service

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"retail-backend/internal/model"
	"retail-backend/internal/repository"
	"retail-backend/internal/store"
)

// CatalogService — справочники и товары.
type CatalogService struct {
	Store      *store.Store
	Categories repository.CategoryRepo
	Brands     repository.BrandRepo
	Products   repository.ProductRepo
	Audit      repository.AuditRepo
}

func (s *CatalogService) ListCategories(ctx context.Context) []model.Category {
	return s.Categories.List(ctx, s.Store.PG)
}

func (s *CatalogService) CreateCategory(ctx context.Context, code, name string, parentID *int64, markedDefault bool, actorID int64, ip, ua string) (int64, error) {
	if code == "" || name == "" {
		return 0, BadRequest("code/name required")
	}
	id, err := s.Categories.Create(ctx, s.Store.PG, code, name, parentID, markedDefault)
	if err != nil {
		return 0, Conflict("duplicate code")
	}
	s.Audit.Log(ctx, s.Store.PG, &actorID, "category.create", "Создание категории", "category", &id,
		map[string]string{"code": code, "name": name}, ip, ua, true, "")
	return id, nil
}

func (s *CatalogService) ListBrands(ctx context.Context) []model.Brand {
	return s.Brands.List(ctx, s.Store.PG)
}

func (s *CatalogService) CreateBrand(ctx context.Context, name, country string) (int64, error) {
	if name == "" {
		return 0, BadRequest("name required")
	}
	id, err := s.Brands.Create(ctx, s.Store.PG, name, country)
	if err != nil {
		return 0, Conflict("duplicate name")
	}
	return id, nil
}

func (s *CatalogService) ListProducts(ctx context.Context, f repository.ProductFilter) []model.Product {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	return s.Products.List(ctx, s.Store.PG, f)
}

func (s *CatalogService) ByCode(ctx context.Context, code string, orgID int64) (model.Product, error) {
	if strings.TrimSpace(code) == "" {
		return model.Product{}, BadRequest("code required")
	}
	p, err := s.Products.ByCode(ctx, s.Store.PG, strings.TrimSpace(code), orgID)
	if err != nil {
		return p, NotFound("not found")
	}
	return p, nil
}

func (s *CatalogService) CreateProduct(ctx context.Context, in repository.CreateInput, actorID int64, ip, ua string) (int64, error) {
	if in.SKU == "" || in.Name == "" {
		return 0, BadRequest("sku/name required")
	}
	if in.VATRate != nil && (*in.VATRate < 0 || *in.VATRate > 30) {
		return 0, BadRequest("vat_rate 0..30")
	}
	st := in.StatusCode
	if st == "" {
		st = "ACTIVE"
	}
	pt := in.ProductType
	if pt == "" {
		pt = "GOODS"
	}
	if pt != "GOODS" && pt != "SERVICE" {
		return 0, BadRequest("product_type GOODS/SERVICE")
	}
	var id int64
	err := s.Store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = s.Products.Create(ctx, tx, in, st, pt)
		return err
	})
	if err != nil {
		return 0, Conflict("create failed (duplicate sku/gtin?)")
	}
	s.Audit.Log(ctx, s.Store.PG, &actorID, "product.create", "Создание товара", "product", &id, in, ip, ua, true, "")
	return id, nil
}

func (s *CatalogService) UpdateProduct(ctx context.Context, id int64, raw map[string]interface{}) error {
	if v, ok := raw["vat_rate"].(float64); ok && (v < 0 || v > 30) {
		return BadRequest("vat_rate 0..30")
	}
	if v, ok := raw["product_type"].(string); ok && v != "" && v != "GOODS" && v != "SERVICE" {
		return BadRequest("product_type GOODS/SERVICE")
	}
	ok, err := s.Products.Update(ctx, s.Store.PG, id, raw)
	if err != nil {
		return BadRequest("update failed")
	}
	if !ok {
		return NotFound("not found")
	}
	return nil
}

func (s *CatalogService) DeleteProduct(ctx context.Context, id int64, actorID int64, ip, ua string) error {
	ok, err := s.Products.Deactivate(ctx, s.Store.PG, id)
	if err != nil || !ok {
		return NotFound("not found")
	}
	s.Audit.Log(ctx, s.Store.PG, &actorID, "product.delete", "Мягкое удаление товара", "product", &id, nil, ip, ua, true, "")
	return nil
}

func (s *CatalogService) ListPrices(ctx context.Context, productID int64) []model.Price {
	return s.Products.Prices(ctx, s.Store.PG, productID)
}

func (s *CatalogService) AddPrice(ctx context.Context, productID, priceTypeID int64, price float64, validFrom string) error {
	if priceTypeID == 0 || price < 0 {
		return BadRequest("price_type_id/price>=0 required")
	}
	if err := s.Products.AddPrice(ctx, s.Store.PG, productID, priceTypeID, price, validFrom); err != nil {
		return BadRequest("add price failed")
	}
	return nil
}

func (s *CatalogService) ListPriceTypes(ctx context.Context, orgID int64) ([]model.PriceType, error) {
	if orgID == 0 {
		return nil, BadRequest("org_id required")
	}
	return s.Products.PriceTypes(ctx, s.Store.PG, orgID), nil
}
