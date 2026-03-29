package usecase

import (
	"context"
	"fmt"

	"ap2/payment-service/internal/domain"
)

// GetPaymentByOrder retrieves the payment associated with an order ID.
type GetPaymentByOrder struct {
	repo PaymentRepository
}

func NewGetPaymentByOrder(repo PaymentRepository) *GetPaymentByOrder {
	return &GetPaymentByOrder{repo: repo}
}

func (uc *GetPaymentByOrder) Execute(ctx context.Context, orderID string) (*domain.Payment, error) {
	if orderID == "" {
		return nil, fmt.Errorf("order_id is required")
	}
	return uc.repo.GetByOrderID(ctx, orderID)
}
