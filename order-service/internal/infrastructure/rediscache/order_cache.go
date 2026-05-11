package rediscache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"ap2/order-service/internal/domain"
)

const keyPrefix = "order:"

// Store implements usecase.OrderCache using Redis.
type Store struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func (c *Store) key(id string) string {
	return keyPrefix + id
}

func (c *Store) Get(ctx context.Context, id string) (*domain.Order, error) {
	if c == nil || c.rdb == nil {
		return nil, nil
	}
	s, err := c.rdb.Get(ctx, c.key(id)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var o domain.Order
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		_ = c.rdb.Del(ctx, c.key(id))
		return nil, nil
	}
	return &o, nil
}

func (c *Store) Set(ctx context.Context, o *domain.Order, ttl time.Duration) error {
	if c == nil || c.rdb == nil || o == nil {
		return nil
	}
	b, err := json.Marshal(o)
	if err != nil {
		return fmt.Errorf("marshal order: %w", err)
	}
	if err := c.rdb.Set(ctx, c.key(o.ID), b, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

func (c *Store) Delete(ctx context.Context, id string) error {
	if c == nil || c.rdb == nil || id == "" {
		return nil
	}
	if err := c.rdb.Del(ctx, c.key(id)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}
