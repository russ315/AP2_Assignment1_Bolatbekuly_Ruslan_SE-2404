package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ap2/order-service/internal/domain"
)

// OrderRepository implements usecase.OrderRepository.
type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) GetByID(ctx context.Context, id string) (*domain.Order, error) {
	const q = `
		SELECT id, customer_id, item_name, amount, status, created_at
		FROM orders
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, q, id)
	var o domain.Order
	err := row.Scan(&o.ID, &o.CustomerID, &o.ItemName, &o.Amount, &o.Status, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	return &o, nil
}

func (r *OrderRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	const q = `UPDATE orders SET status = $2 WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id, status)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("order not found")
	}
	return nil
}

// CreatePendingWithIdempotency creates a new pending order or returns an existing one for the idempotency key.
func (r *OrderRepository) CreatePendingWithIdempotency(
	ctx context.Context,
	idempotencyKey *string,
	customerID, itemName string,
	amount int64,
) (*domain.Order, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if idempotencyKey != nil && *idempotencyKey != "" {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(abs(hashtext($1::text)))`, *idempotencyKey); err != nil {
			return nil, false, fmt.Errorf("advisory lock: %w", err)
		}

		var existingID string
		err := tx.QueryRowContext(ctx, `SELECT order_id FROM idempotency_keys WHERE key = $1`, *idempotencyKey).Scan(&existingID)
		if err == nil {
			var o domain.Order
			err = tx.QueryRowContext(ctx, `
				SELECT id, customer_id, item_name, amount, status, created_at
				FROM orders WHERE id = $1`, existingID,
			).Scan(&o.ID, &o.CustomerID, &o.ItemName, &o.Amount, &o.Status, &o.CreatedAt)
			if err != nil {
				return nil, false, fmt.Errorf("load idempotent order: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return nil, false, err
			}
			return &o, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, fmt.Errorf("idempotency lookup: %w", err)
		}
	}

	now := time.Now().UTC()
	o := &domain.Order{
		ID:         uuid.NewString(),
		CustomerID: customerID,
		ItemName:   itemName,
		Amount:     amount,
		Status:     domain.StatusPending,
		CreatedAt:  now,
	}

	const insertOrder = `
		INSERT INTO orders (id, customer_id, item_name, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	if _, err := tx.ExecContext(ctx, insertOrder, o.ID, o.CustomerID, o.ItemName, o.Amount, o.Status, o.CreatedAt); err != nil {
		return nil, false, fmt.Errorf("insert order: %w", err)
	}

	if idempotencyKey != nil && *idempotencyKey != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO idempotency_keys (key, order_id) VALUES ($1, $2)`, *idempotencyKey, o.ID); err != nil {
			return nil, false, fmt.Errorf("insert idempotency: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return o, false, nil
}
