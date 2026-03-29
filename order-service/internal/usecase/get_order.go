package usecase

import (
	"context"

	"ap2/order-service/internal/domain"
)

// GetOrder loads an order by identifier.
type GetOrder struct {
	orders OrderRepository
}

func NewGetOrder(orders OrderRepository) *GetOrder {
	return &GetOrder{orders: orders}
}

func (uc *GetOrder) Execute(ctx context.Context, id string) (*domain.Order, error) {
	o, err := uc.orders.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrOrderNotFound
	}
	return o, nil
}
