package usecase

import (
	"context"

	"ap2/payment-service/internal/domain"
)

// PaymentRepository persists payment records (outbound port).
type PaymentRepository interface {
	Save(ctx context.Context, p *domain.Payment) error
	GetByOrderID(ctx context.Context, orderID string) (*domain.Payment, error)
}
