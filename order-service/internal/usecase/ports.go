package usecase

import (
	"context"
	"time"

	"ap2/order-service/internal/domain"
)

// OrderRepository persists orders (outbound port).
type OrderRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Order, error)
	UpdateStatus(ctx context.Context, id string, status string) error
	CreatePendingWithIdempotency(ctx context.Context, idempotencyKey *string, customerID, itemName string, amount int64) (*domain.Order, bool, error)
}

// OrderCache is optional cache-aside storage for order reads (Redis).
// Implementations return (nil, nil) on cache miss.
type OrderCache interface {
	Get(ctx context.Context, id string) (*domain.Order, error)
	Set(ctx context.Context, o *domain.Order, ttl time.Duration) error
	Delete(ctx context.Context, id string) error
}

// PaymentAuthorizer calls the Payment Service over gRPC (outbound port).
type PaymentAuthorizer interface {
	Authorize(ctx context.Context, orderID string, amount int64, customerEmail string) (transactionID string, status string, err error)
	GetStatus(ctx context.Context, orderID string) (status string, err error)
}
