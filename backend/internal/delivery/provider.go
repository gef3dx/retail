package delivery

import (
	"context"
	"fmt"
	"time"
)

// Provider — внешняя служба доставки (трекинг, создание отправлений).
// Своя доставка (курьеры) работает без провайдера; внешние типы — через него.
type Provider interface {
	Code() string
	// CreateShipment регистрирует отправление, возвращает трек-номер.
	CreateShipment(ctx context.Context, creds map[string]string, orderID int64, address string) (string, error)
}

// Emulator — встроенный эмулятор служб (только разработка).
type Emulator struct{}

func (Emulator) Code() string { return "DELIVERY_EMULATOR" }

func (Emulator) CreateShipment(_ context.Context, _ map[string]string, orderID int64, _ string) (string, error) {
	return fmt.Sprintf("EMB-%d-%d", orderID, time.Now().Unix()%100000), nil
}
