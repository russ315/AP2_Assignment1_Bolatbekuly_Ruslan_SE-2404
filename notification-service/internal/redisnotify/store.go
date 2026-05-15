package redisnotify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const sentValue = "SENT"

// Store tracks per-payment notification completion in Redis.
type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(rdb *redis.Client, ttl time.Duration) *Store {
	return &Store{rdb: rdb, ttl: ttl}
}

func (s *Store) key(paymentID string) string {
	return "notify:payment:" + paymentID
}

// AlreadySent reports whether this payment_id was fully processed (email sent).
func (s *Store) AlreadySent(ctx context.Context, paymentID string) (bool, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return false, fmt.Errorf("empty payment_id")
	}
	v, err := s.rdb.Get(ctx, s.key(paymentID)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis get: %w", err)
	}
	return v == sentValue, nil
}

// MarkSent records successful notification for payment_id.
func (s *Store) MarkSent(ctx context.Context, paymentID string) error {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return fmt.Errorf("empty payment_id")
	}
	if err := s.rdb.Set(ctx, s.key(paymentID), sentValue, s.ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}
