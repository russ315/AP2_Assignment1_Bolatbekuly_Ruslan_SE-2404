package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"ap2/payment-service/internal/domain"
)

// AuthorizePaymentInput carries data required to authorize a payment.
type AuthorizePaymentInput struct {
	OrderID       string
	Amount        int64
	CustomerEmail string
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
	pub  PaymentCompletedPublisher // optional; nil skips broker publish
}

func NewAuthorizePayment(repo PaymentRepository, pub PaymentCompletedPublisher) *AuthorizePayment {
	return &AuthorizePayment{repo: repo, pub: pub}
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

	if p.Status == domain.StatusAuthorized && uc.pub != nil {
		email := strings.TrimSpace(in.CustomerEmail)
		if email == "" {
			return nil, fmt.Errorf("customer_email is required for authorized payments")
		}
		err := uc.pub.PublishPaymentCompleted(ctx, PaymentCompletedEvent{
			EventID:       p.ID,
			OrderID:       p.OrderID,
			AmountCents:   p.Amount,
			CustomerEmail: email,
			Status:        p.Status,
		})
		if err != nil {
			return nil, fmt.Errorf("publish payment completed event: %w", err)
		}
	}

	return &AuthorizePaymentOutput{
		PaymentID:     p.ID,
		TransactionID: p.TransactionID,
		Status:        p.Status,
		Amount:        p.Amount,
		CreatedAt:     p.CreatedAt,
	}, nil
}
