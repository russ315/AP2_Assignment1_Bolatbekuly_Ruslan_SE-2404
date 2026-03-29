package usecase

import (
	"context"
	"fmt"
	"strings"

	"ap2/order-service/internal/domain"
)

const paymentStatusAuthorized = "Authorized"
const paymentStatusDeclined = "Declined"

// CreateOrderInput is the input for placing an order.
type CreateOrderInput struct {
	CustomerID       string
	ItemName         string
	Amount           int64
	IdempotencyKey   *string
}

// CreateOrder coordinates persistence and payment authorization.
type CreateOrder struct {
	orders   OrderRepository
	payments PaymentAuthorizer
}

func NewCreateOrder(orders OrderRepository, payments PaymentAuthorizer) *CreateOrder {
	return &CreateOrder{orders: orders, payments: payments}
}

func (uc *CreateOrder) Execute(ctx context.Context, in CreateOrderInput) (*domain.Order, error) {
	if in.Amount <= 0 {
		return nil, ErrInvalidInput
	}
	customerID := strings.TrimSpace(in.CustomerID)
	itemName := strings.TrimSpace(in.ItemName)
	if customerID == "" || itemName == "" {
		return nil, ErrInvalidInput
	}

	order, _, err := uc.orders.CreatePendingWithIdempotency(ctx, in.IdempotencyKey, customerID, itemName, in.Amount)
	if err != nil {
		return nil, err
	}

	switch order.Status {
	case domain.StatusPaid, domain.StatusFailed, domain.StatusCancelled:
		return order, nil
	}

	// Pending: complete or reconcile payment
	_, payStatus, err := uc.payments.Authorize(ctx, order.ID, order.Amount)
	if err != nil {
		if err == ErrPaymentAlreadyRecorded {
			return uc.reconcileFromPaymentService(ctx, order)
		}
		if err == ErrPaymentUnavailable {
			// Leave order Pending: payment outcome is unknown (timeout / unreachable).
			// Client may retry checkout or cancel while still Pending.
			fresh, _ := uc.orders.GetByID(ctx, order.ID)
			if fresh != nil {
				order = fresh
			}
			return order, ErrPaymentUnavailable
		}
		_ = uc.orders.UpdateStatus(ctx, order.ID, domain.StatusFailed)
		fresh, _ := uc.orders.GetByID(ctx, order.ID)
		if fresh != nil {
			order = fresh
		} else {
			order.Status = domain.StatusFailed
		}
		return order, err
	}

	newStatus := domain.StatusFailed
	if payStatus == paymentStatusAuthorized {
		newStatus = domain.StatusPaid
	} else if payStatus == paymentStatusDeclined {
		newStatus = domain.StatusFailed
	}

	if err := uc.orders.UpdateStatus(ctx, order.ID, newStatus); err != nil {
		return nil, err
	}
	order.Status = newStatus

	return order, nil
}

func (uc *CreateOrder) reconcileFromPaymentService(ctx context.Context, order *domain.Order) (*domain.Order, error) {
	st, err := uc.payments.GetStatus(ctx, order.ID)
	if err != nil {
		if err == ErrPaymentNotFound {
			return nil, fmt.Errorf("payment record missing after duplicate authorization")
		}
		if err == ErrPaymentUnavailable {
			fresh, _ := uc.orders.GetByID(ctx, order.ID)
			if fresh != nil {
				return fresh, ErrPaymentUnavailable
			}
			return order, ErrPaymentUnavailable
		}
		return nil, err
	}

	newStatus := domain.StatusFailed
	if st == paymentStatusAuthorized {
		newStatus = domain.StatusPaid
	} else if st == paymentStatusDeclined {
		newStatus = domain.StatusFailed
	}

	if err := uc.orders.UpdateStatus(ctx, order.ID, newStatus); err != nil {
		return nil, err
	}
	order.Status = newStatus
	return order, nil
}
