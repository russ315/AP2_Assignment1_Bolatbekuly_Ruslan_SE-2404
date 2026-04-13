package grpc

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ap2/order-service/internal/domain"
	orderv1 "ap2/order-service/internal/transport/grpc/proto/orderv1"
)

// OrderUpdateSubscriber represents a client subscribed to order updates
type OrderUpdateSubscriber struct {
	stream  orderv1.OrderService_SubscribeToOrderUpdatesServer
	orderID string
	done    chan struct{}
}

// OrderServer implements the OrderService gRPC interface
type OrderServer struct {
	orderv1.UnimplementedOrderServiceServer
	db     *sql.DB
	mu     sync.RWMutex
	subs   map[string][]*OrderUpdateSubscriber // orderID -> subscribers
	ctx    context.Context
	cancel context.CancelFunc
}

// NewOrderServer creates a new order gRPC server
func NewOrderServer(db *sql.DB) *OrderServer {
	ctx, cancel := context.WithCancel(context.Background())
	server := &OrderServer{
		db:     db,
		subs:   make(map[string][]*OrderUpdateSubscriber),
		ctx:    ctx,
		cancel: cancel,
	}

	// Start database monitoring
	go server.monitorDatabaseChanges()

	return server
}

// SubscribeToOrderUpdates implements server-side streaming for order updates
func (s *OrderServer) SubscribeToOrderUpdates(req *orderv1.OrderRequest, stream orderv1.OrderService_SubscribeToOrderUpdatesServer) error {
	if req.OrderId == "" {
		return status.Error(codes.InvalidArgument, "order_id is required")
	}

	// Verify order exists
	order, err := s.getOrder(req.OrderId)
	if err != nil {
		return status.Error(codes.NotFound, fmt.Sprintf("order not found: %s", req.OrderId))
	}

	subscriber := &OrderUpdateSubscriber{
		stream:  stream,
		orderID: req.OrderId,
		done:    make(chan struct{}),
	}

	// Add subscriber
	s.mu.Lock()
	s.subs[req.OrderId] = append(s.subs[req.OrderId], subscriber)
	s.mu.Unlock()

	log.Printf("Client subscribed to order updates: %s", req.OrderId)

	// Send initial order status
	if err := stream.Send(&orderv1.OrderStatusUpdate{
		OrderId:    order.ID,
		CustomerId: order.CustomerID,
		ItemName:   order.ItemName,
		Amount:     order.Amount,
		Status:     order.Status,
		UpdatedAt:  timestamppb.Now(),
	}); err != nil {
		return err
	}

	// Keep stream alive and wait for updates or disconnection
	<-subscriber.done
	s.removeSubscriber(subscriber)

	return nil
}

// monitorDatabaseChanges polls the database for order status changes
func (s *OrderServer) monitorDatabaseChanges() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	lastStatuses := make(map[string]string)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkForUpdates(lastStatuses)
		}
	}
}

// checkForUpdates checks database for order status changes
func (s *OrderServer) checkForUpdates(lastStatuses map[string]string) {
	s.mu.RLock()
	orderIDs := make([]string, 0, len(s.subs))
	for orderID := range s.subs {
		orderIDs = append(orderIDs, orderID)
	}
	s.mu.RUnlock()

	for _, orderID := range orderIDs {
		order, err := s.getOrder(orderID)
		if err != nil {
			continue
		}

		lastStatus, exists := lastStatuses[orderID]
		if !exists {
			lastStatuses[orderID] = order.Status
			continue
		}

		if order.Status != lastStatus {
			lastStatuses[orderID] = order.Status
			s.notifySubscribers(orderID, order)
		}
	}
}

// getOrder retrieves order from database
func (s *OrderServer) getOrder(orderID string) (*domain.Order, error) {
	var order domain.Order
	query := `SELECT id, customer_id, item_name, amount, status, created_at FROM orders WHERE id = $1`

	err := s.db.QueryRow(query, orderID).Scan(
		&order.ID,
		&order.CustomerID,
		&order.ItemName,
		&order.Amount,
		&order.Status,
		&order.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &order, nil
}

// notifySubscribers sends updates to all subscribers of an order
func (s *OrderServer) notifySubscribers(orderID string, order *domain.Order) {
	s.mu.RLock()
	subscribers := s.subs[orderID]
	s.mu.RUnlock()

	update := &orderv1.OrderStatusUpdate{
		OrderId:    order.ID,
		CustomerId: order.CustomerID,
		ItemName:   order.ItemName,
		Amount:     order.Amount,
		Status:     order.Status,
		UpdatedAt:  timestamppb.Now(),
	}

	for _, sub := range subscribers {
		go func(subscriber *OrderUpdateSubscriber) {
			if err := subscriber.stream.Send(update); err != nil {
				log.Printf("Failed to send update to subscriber: %v", err)
				close(subscriber.done)
			}
		}(sub)
	}
}

// removeSubscriber removes a subscriber from the list
func (s *OrderServer) removeSubscriber(subscriber *OrderUpdateSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subs := s.subs[subscriber.orderID]
	for i, sub := range subs {
		if sub == subscriber {
			s.subs[subscriber.orderID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}

	if len(s.subs[subscriber.orderID]) == 0 {
		delete(s.subs, subscriber.orderID)
	}
}

// Stop stops the order server
func (s *OrderServer) Stop() {
	s.cancel()
}

// RegisterOrderServer registers the order service with the gRPC server
func RegisterOrderServer(grpcServer *grpc.Server, server *OrderServer) {
	orderv1.RegisterOrderServiceServer(grpcServer, server)
}
