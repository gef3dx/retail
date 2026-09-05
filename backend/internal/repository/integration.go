package repository

import (
	"context"
	"encoding/json"

	"retail-backend/internal/provider"
)

// IntegrationRepo — хранилище настроек провайдеров (секреты шифруются).
type IntegrationRepo struct{}

// storedCreds — формат credentials в БД: {key: "enc:base64" | "plain:..."}.
// Секреты (по KeySpec.Secret) хранятся шифрованными с префиксом "enc:".
func encryptMap(in map[string]string, specs []provider.KeySpec) map[string]string {
	secret := map[string]bool{}
	for _, k := range specs {
		if k.Secret {
			secret[k.Key] = true
		}
	}
	out := map[string]string{}
	for k, v := range in {
		if v == "" {
			continue
		}
		if secret[k] {
			if enc, err := provider.Encrypt(v); err == nil {
				out[k] = "enc:" + enc
				continue
			}
		}
		out[k] = "plain:" + v
	}
	return out
}

func decryptMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		switch {
		case len(v) > 4 && v[:4] == "enc:":
			if dec, err := provider.Decrypt(v[4:]); err == nil {
				out[k] = dec
			}
		case len(v) > 6 && v[:6] == "plain:":
			out[k] = v[6:]
		default:
			out[k] = v // legacy / совместимость
		}
	}
	return out
}

// Get возвращает расшифрованные credentials + enabled (found=false если строки нет).
func (IntegrationRepo) Get(ctx context.Context, db DBTX, orgID int64, code string) (map[string]string, bool, bool) {
	var raw []byte
	var enabled bool
	err := db.QueryRow(ctx, `
		SELECT credentials, enabled FROM integration_settings
		WHERE organization_id=$1 AND provider_code=$2`, orgID, code).Scan(&raw, &enabled)
	if err != nil {
		return map[string]string{}, true, false
	}
	var stored map[string]string
	_ = json.Unmarshal(raw, &stored)
	return decryptMap(stored), enabled, true
}

// Upsert сохраняет (шифруя секреты). Пустые значения удаляют ключ.
func (IntegrationRepo) Upsert(ctx context.Context, db DBTX, reg *provider.Registry, orgID int64, code string, creds map[string]string, enabled *bool) error {
	p := reg.ByCode(code)
	if p == nil {
		return errNotFound
	}
	existing, _, _ := (IntegrationRepo{}).Get(ctx, db, orgID, code)
	for k, v := range creds {
		if v == "" {
			delete(existing, k)
		} else {
			existing[k] = v
		}
	}
	enc, err := json.Marshal(encryptMap(existing, p.Keys()))
	if err != nil {
		return err
	}
	if enabled == nil {
		def := true
		enabled = &def
	}
	_, err = db.Exec(ctx, `
		INSERT INTO integration_settings(organization_id, provider_code, credentials, enabled, updated_at)
		VALUES($1,$2,$3,$4,NOW())
		ON CONFLICT (organization_id, provider_code)
		DO UPDATE SET credentials=$3, enabled=$4, updated_at=NOW()`,
		orgID, code, enc, *enabled)
	return err
}

// Clear удаляет настройки провайдера (возврат к дефолту).
func (IntegrationRepo) Clear(ctx context.Context, db DBTX, orgID int64, code string) {
	_, _ = db.Exec(ctx, `DELETE FROM integration_settings WHERE organization_id=$1 AND provider_code=$2`, orgID, code)
}

// Statuses вычисляет статусы всех провайдеров для организации.
func (IntegrationRepo) Statuses(ctx context.Context, db DBTX, reg *provider.Registry, orgID int64) []provider.ProviderStatus {
	var out []provider.ProviderStatus
	for _, p := range reg.All() {
		creds, enabled, found := (IntegrationRepo{}).Get(ctx, db, orgID, p.Code())
		if !found {
			enabled = true
		}
		st := provider.StatusInactive
		if !enabled {
			st = provider.StatusDisabled
		} else if p.IsConfigured(creds) {
			st = provider.StatusActive
		}
		has := map[string]bool{}
		var missing []string
		for _, k := range p.Keys() {
			has[k.Key] = creds[k.Key] != ""
			if k.Required && creds[k.Key] == "" {
				missing = append(missing, k.Key)
			}
		}
		out = append(out, provider.ProviderStatus{
			Code: p.Code(), Name: p.Name(), Kind: p.Kind(), Status: st,
			Enabled: enabled, Emulator: p.Emulator(), Keys: p.Keys(),
			HasValue: has, Missing: missing,
		})
	}
	return out
}
