package email

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"time"
)

// SimulatedSender logs emails, adds latency, and randomly fails (for retry testing).
type SimulatedSender struct {
	MinLatency time.Duration
	MaxLatency time.Duration
	FailRate   float64 // 0..1
}

func NewSimulated(minLat, maxLat time.Duration, failRate float64) *SimulatedSender {
	if minLat <= 0 {
		minLat = 50 * time.Millisecond
	}
	if maxLat < minLat {
		maxLat = minLat + 100*time.Millisecond
	}
	if failRate < 0 {
		failRate = 0
	}
	if failRate > 1 {
		failRate = 1
	}
	return &SimulatedSender{MinLatency: minLat, MaxLatency: maxLat, FailRate: failRate}
}

func (s *SimulatedSender) SendPaymentReceipt(ctx context.Context, in SendInput) error {
	d := s.MinLatency
	if s.MaxLatency > s.MinLatency {
		d += time.Duration(rand.Int63n(int64(s.MaxLatency - s.MinLatency)))
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
	}
	if rand.Float64() < s.FailRate {
		return errors.New("simulated provider transient failure")
	}
	log.Printf("[SimulatedEmail] payment_id=%s order_id=%s to=%s amount_cents=%d",
		in.PaymentID, in.OrderID, in.ToEmail, in.AmountCents)
	return nil
}
