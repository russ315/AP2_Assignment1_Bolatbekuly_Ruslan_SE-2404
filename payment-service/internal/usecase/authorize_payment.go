package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ap2/payment-service/internal/domain"
)

// AuthorizePaymentInput carries data required to authorize a payment.
type AuthorizePaymentInput struct {
	OrderID string
	Amount  int64
}

// AuthorizePaymentOutput is the result of an authorization attempt.
type AuthorizePaymentOutput struct {
	PaymentID       string
	TransactionID   string
	Status          string
	Amount          int64
	CreatedAt       time.Time
}

// AuthorizePayment processes a payment authorization request.
type AuthorizePayment struct {
	repo PaymentRepository
}

func NewAuthorizePayment(repo PaymentRepository) *AuthorizePayment {
	return &AuthorizePayment{repo: repo}
}

func (uc *AuthorizePayment) Execute(ctx context.Context, in AuthorizePaymentInput) (*AuthorizePaymentOutput, error) {
	if in.Amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than zero")
	}
	if in.OrderID == "" {
		return nil, fmt.Errorf("order_id is required")
	}

	existing, err := uc.repo.GetByOrderID(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("payment already exists for this order")
	}

	now := time.Now().UTC()
	paymentID := uuid.NewString()
	txnID := uuid.NewString()

	status := domain.StatusAuthorized
	if in.Amount > domain.MaxAuthorizedAmountCents {
		status = domain.StatusDeclined
	}

	p := &domain.Payment{
		ID:            paymentID,
		OrderID:       in.OrderID,
		TransactionID: txnID,
		Amount:        in.Amount,
		Status:        status,
		CreatedAt:     now,
	}

	if err := uc.repo.Save(ctx, p); err != nil {
		return nil, err
	}

	return &AuthorizePaymentOutput{
		PaymentID:     p.ID,
		TransactionID: p.TransactionID,
		Status:        p.Status,
		Amount:        p.Amount,
		CreatedAt:     p.CreatedAt,
	}, nil
}
