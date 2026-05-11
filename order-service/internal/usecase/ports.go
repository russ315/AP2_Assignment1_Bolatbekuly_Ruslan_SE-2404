package usecase

import (
	"context"

	"ap2/order-service/internal/domain"
)

// OrderRepository persists orders (outbound port).
type OrderRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Order, error)
	UpdateStatus(ctx context.Context, id string, status string) error
	CreatePendingWithIdempotency(ctx context.Context, idempotencyKey *string, customerID, itemName string, amount int64) (*domain.Order, bool, error)
}

// PaymentAuthorizer calls the Payment Service over gRPC (outbound port).
type PaymentAuthorizer interface {
	Authorize(ctx context.Context, orderID string, amount int64, customerEmail string) (transactionID string, status string, err error)
	GetStatus(ctx context.Context, orderID string) (status string, err error)
}
