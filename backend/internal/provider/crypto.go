package provider

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
)

// Шифрование секретов в integration_settings (AES-256-GCM).
// Ключ: SETTINGS_ENC_KEY (base64 32 байта или сырая строка 32 байта).
// Без ключа — dev-режим с фиксированным ключом (громкое предупреждение в логе,
// НЕ использовать в проде: секреты лежат фактически в открытом виде).

var gcm cipher.AEAD

func init() {
	key := loadKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("provider crypto: %v", err))
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("provider crypto: %v", err))
	}
	gcm = g
}

func loadKey() []byte {
	if raw := os.Getenv("SETTINGS_ENC_KEY"); raw != "" {
		if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
			return b
		}
		if len(raw) == 32 {
			return []byte(raw)
		}
		slog.Warn("SETTINGS_ENC_KEY must be base64/32 bytes, using dev key")
	}
	slog.Warn("SETTINGS_ENC_KEY not set: using dev-only key, DO NOT use in production")
	return []byte("dev-only-key-32-bytes-for-retail")
}

// Encrypt шифрует plaintext, возвращает base64(nonce|ciphertext).
func Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt расшифровывает base64(nonce|ciphertext).
func Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
