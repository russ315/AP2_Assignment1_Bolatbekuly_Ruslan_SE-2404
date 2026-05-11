package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"ap2/notification-service/internal/email"
	"ap2/notification-service/internal/postgres"
	"ap2/notification-service/internal/redisnotify"
)

// Payload matches JSON from payment-service (RabbitMQ body).
type Payload struct {
	EventID       string `json:"event_id"`
	PaymentID     string `json:"payment_id"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	CustomerEmail string `json:"customer_email"`
	Status        string `json:"status"`
}

var (
	ErrPoison       = errors.New("poison message: no retry")
	ErrForceDLQDemo = errors.New("forced dlq demo")
	ErrTransient    = errors.New("transient failure: retry")
)

// Processor runs background notification handling (Redis idempotency + email adapter + retries).
type Processor struct {
	PG     *postgres.Store
	Redis  *redisnotify.Store
	Sender email.Sender
	// DLQDemoOrderID when non-empty forces dead-letter for matching order_id (bonus demo).
	DLQDemoOrderID string
	MaxAttempts    int
	BackoffBase    time.Duration
}

func (p *Processor) Handle(ctx context.Context, body []byte) error {
	var pl Payload
	if err := json.Unmarshal(body, &pl); err != nil {
		return fmt.Errorf("%w: %v", ErrPoison, err)
	}
	if strings.TrimSpace(pl.EventID) == "" {
		return fmt.Errorf("%w: missing event_id", ErrPoison)
	}

	paymentID := strings.TrimSpace(pl.PaymentID)
	if paymentID == "" {
		paymentID = strings.TrimSpace(pl.EventID)
	}

	dup, err := p.PG.WasProcessed(ctx, pl.EventID)
	if err != nil {
		return fmt.Errorf("%w: idempotency store: %v", ErrTransient, err)
	}
	if dup {
		return nil
	}

	sent, err := p.Redis.AlreadySent(ctx, paymentID)
	if err != nil {
		return fmt.Errorf("%w: redis idempotency: %v", ErrTransient, err)
	}
	if sent {
		return nil
	}

	if p.DLQDemoOrderID != "" && strings.TrimSpace(pl.OrderID) == p.DLQDemoOrderID {
		return ErrForceDLQDemo
	}

	in := email.SendInput{
		ToEmail:     strings.TrimSpace(pl.CustomerEmail),
		OrderID:     strings.TrimSpace(pl.OrderID),
		PaymentID:   paymentID,
		AmountCents: pl.Amount,
		Status:      strings.TrimSpace(pl.Status),
	}
	if in.ToEmail == "" {
		return fmt.Errorf("%w: missing customer_email", ErrPoison)
	}

	var lastErr error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := p.BackoffBase * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		lastErr = p.Sender.SendPaymentReceipt(ctx, in)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return fmt.Errorf("%w: %v", ErrTransient, lastErr)
	}

	if err := p.PG.RecordProcessed(ctx, pl.EventID); err != nil {
		return fmt.Errorf("%w: record processed: %v", ErrTransient, err)
	}
	if err := p.Redis.MarkSent(ctx, paymentID); err != nil {
		return fmt.Errorf("%w: redis mark sent: %v", ErrTransient, err)
	}

	amt := formatUSD(pl.Amount)
	log.Printf("[Notification] Sent email to %s for Order #%s. Amount: %s", pl.CustomerEmail, pl.OrderID, amt)
	return nil
}

func formatUSD(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	dollars := cents / 100
	fr := cents % 100
	s := fmt.Sprintf("$%d.%02d", dollars, fr)
	if neg {
		return "-" + s
	}
	return s
}
