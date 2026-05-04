package usecase

import "context"

// PaymentCompletedPublisher emits payment.completed domain events to the message broker (outbound port).
type PaymentCompletedPublisher interface {
	PublishPaymentCompleted(ctx context.Context, e PaymentCompletedEvent) error
}

// PaymentCompletedEvent is the payload published after a successful DB commit for an authorized payment.
type PaymentCompletedEvent struct {
	EventID       string
	OrderID       string
	AmountCents   int64
	CustomerEmail string
	Status        string
}
