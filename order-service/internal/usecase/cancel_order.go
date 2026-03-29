package usecase

import (
	"context"

	"ap2/order-service/internal/domain"
)

// CancelOrder cancels a pending order.
type CancelOrder struct {
	orders OrderRepository
}

func NewCancelOrder(orders OrderRepository) *CancelOrder {
	return &CancelOrder{orders: orders}
}

func (uc *CancelOrder) Execute(ctx context.Context, id string) (*domain.Order, error) {
	o, err := uc.orders.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrOrderNotFound
	}
	if o.Status != domain.StatusPending {
		return nil, ErrCancelNotAllowed
	}
	if err := uc.orders.UpdateStatus(ctx, id, domain.StatusCancelled); err != nil {
		return nil, err
	}
	o.Status = domain.StatusCancelled
	return o, nil
}
