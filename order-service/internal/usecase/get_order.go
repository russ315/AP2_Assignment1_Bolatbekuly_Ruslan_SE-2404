package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ap2/order-service/internal/domain"
)

// GetOrder loads an order by identifier (cache-aside when cache is configured).
type GetOrder struct {
	orders OrderRepository
	cache  OrderCache
	ttl    time.Duration
}

// NewGetOrder constructs GetOrder. cache may be nil (no Redis).
func NewGetOrder(orders OrderRepository, cache OrderCache, ttl time.Duration) *GetOrder {
	return &GetOrder{orders: orders, cache: cache, ttl: ttl}
}

func (uc *GetOrder) Execute(ctx context.Context, id string) (*domain.Order, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrOrderNotFound
	}

	if uc.cache != nil {
		cached, err := uc.cache.Get(ctx, id)
		fmt.Printf("cache get for order %s: hit=%t err=%v\n", id, cached != nil, err)
		if err == nil && cached != nil {
			return cached, nil
		}
	}
	fmt.Printf("database get for order %s", id)

	o, err := uc.orders.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrOrderNotFound
	}

	if uc.cache != nil && uc.ttl > 0 {
		_ = uc.cache.Set(ctx, o, uc.ttl)
	}

	return o, nil
}
