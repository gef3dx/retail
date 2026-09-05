package repository

import (
	"context"
	"errors"
)

// ErrNoStock и ErrInsufficientStock — сервисный слой мапит их в 409.
var (
	ErrNoStock           = errors.New("no stock for product")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrAlreadyPosted     = errors.New("already posted")
)

// BalanceRepo — остатки склада (все методы для вызова внутри транзакций).
type BalanceRepo struct{}

// Deduct списывает свободные остатки (quantity - reserved).
func (BalanceRepo) Deduct(ctx context.Context, db DBTX, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		var avail float64
		if err := db.QueryRow(ctx, `
			SELECT COALESCE(quantity,0) - COALESCE(reserved_quantity,0) FROM warehouse_balance
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid).Scan(&avail); err != nil {
			return ErrNoStock
		}
		if avail < qty {
			return ErrInsufficientStock
		}
		if _, err := db.Exec(ctx, `
			UPDATE warehouse_balance SET quantity = quantity - $3, last_updated=NOW()
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid, qty); err != nil {
			return err
		}
	}
	return nil
}

// Add приходует остатки (upsert).
func (BalanceRepo) Add(ctx context.Context, db DBTX, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		if _, err := db.Exec(ctx, `
			INSERT INTO warehouse_balance(warehouse_id, product_id, quantity)
			VALUES($1,$2,$3)
			ON CONFLICT (warehouse_id, product_id) DO UPDATE SET quantity = warehouse_balance.quantity + $3, last_updated=NOW()`,
			warehouseID, pid, qty); err != nil {
			return err
		}
	}
	return nil
}

// Reserve резервирует свободные остатки под заказ.
func (BalanceRepo) Reserve(ctx context.Context, db DBTX, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		res, err := db.Exec(ctx, `
			UPDATE warehouse_balance SET reserved_quantity = reserved_quantity + $3, last_updated=NOW()
			WHERE warehouse_id=$1 AND product_id=$2
			  AND (quantity - reserved_quantity) >= $3`, warehouseID, pid, qty)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return ErrInsufficientStock
		}
	}
	return nil
}

// Release снимает резерв (не ниже нуля).
func (BalanceRepo) Release(ctx context.Context, db DBTX, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		if _, err := db.Exec(ctx, `
			UPDATE warehouse_balance SET reserved_quantity = GREATEST(0, reserved_quantity - $3), last_updated=NOW()
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid, qty); err != nil {
			return err
		}
	}
	return nil
}

// Ship списывает физические остатки и снимает резерв (отгрузка по заказу).
func (BalanceRepo) Ship(ctx context.Context, db DBTX, warehouseID int64, items map[int64]float64) error {
	for pid, qty := range items {
		var physical float64
		if err := db.QueryRow(ctx, `
			SELECT quantity FROM warehouse_balance
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid).Scan(&physical); err != nil || physical < qty {
			return ErrInsufficientStock
		}
		if _, err := db.Exec(ctx, `
			UPDATE warehouse_balance SET quantity = quantity - $3,
				reserved_quantity = GREATEST(0, reserved_quantity - $3), last_updated=NOW()
			WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, pid, qty); err != nil {
			return err
		}
	}
	return nil
}
