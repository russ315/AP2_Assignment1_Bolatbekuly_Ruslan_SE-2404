package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"ap2/notification-service/internal/postgres"
)

// Payload matches the JSON produced by the payment service (decoupled contract).
type Payload struct {
	EventID       string `json:"event_id"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	CustomerEmail string `json:"customer_email"`
	Status        string `json:"status"`
}

var (
	ErrPoison       = errors.New("poison message: no retry")
	ErrForceDLQDemo = errors.New("forced dlq demo")
)

// Process unmarshals the body, applies idempotency, logs the simulated email, or routes poison/DLQ demo cases.
func Process(ctx context.Context, store *postgres.Store, dlqDemoOrderID string, body []byte) error {
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		return fmt.Errorf("%w: %v", ErrPoison, err)
	}
	if strings.TrimSpace(p.EventID) == "" {
		return fmt.Errorf("%w: missing event_id", ErrPoison)
	}

	dup, err := store.WasProcessed(ctx, p.EventID)
	if err != nil {
		return fmt.Errorf("idempotency store: %w", err)
	}
	if dup {
		return nil
	}

	if dlqDemoOrderID != "" && p.OrderID == dlqDemoOrderID {
		return ErrForceDLQDemo
	}

	amt := formatUSD(p.Amount)
	log.Printf("[Notification] Sent email to %s for Order #%s. Amount: %s", p.CustomerEmail, p.OrderID, amt)

	if err := store.RecordProcessed(ctx, p.EventID); err != nil {
		return fmt.Errorf("record processed: %w", err)
	}
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
