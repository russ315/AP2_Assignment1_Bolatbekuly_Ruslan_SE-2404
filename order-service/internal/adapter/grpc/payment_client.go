package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	paymentv1 "ap2/order-service/internal/transport/grpc/proto/paymentv1"
	"ap2/order-service/internal/usecase"
)

// PaymentClient implements the PaymentAuthorizer interface using gRPC
type PaymentClient struct {
	client paymentv1.PaymentServiceClient
	conn   *grpc.ClientConn
}

// NewPaymentClient creates a new gRPC payment client
func NewPaymentClient(address string) (*PaymentClient, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to payment service: %w", err)
	}

	client := paymentv1.NewPaymentServiceClient(conn)

	return &PaymentClient{
		client: client,
		conn:   conn,
	}, nil
}

// Authorize calls the payment service via gRPC
func (p *PaymentClient) Authorize(ctx context.Context, orderID string, amount int64) (transactionID string, paymentStatus string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req := &paymentv1.AuthorizePaymentRequest{
		OrderId: orderID,
		Amount:  amount,
	}

	resp, err := p.client.AuthorizePayment(ctx, req)
	if err != nil {
		if s, ok := status.FromError(err); ok {
			if s.Code() == codes.Internal {
				return "", "", usecase.ErrPaymentUnavailable
			}
			return "", "", fmt.Errorf("payment authorization failed: %w", err)
		}
		return "", "", usecase.ErrPaymentUnavailable
	}

	return resp.TransactionId, resp.Status, nil
}

// GetStatus calls the payment service via gRPC to get payment status
func (p *PaymentClient) GetStatus(ctx context.Context, orderID string) (paymentStatus string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req := &paymentv1.GetPaymentStatusRequest{
		OrderId: orderID,
	}

	resp, err := p.client.GetPaymentStatus(ctx, req)
	if err != nil {
		if s, ok := status.FromError(err); ok {
			if s.Code() == codes.NotFound {
				return "", usecase.ErrPaymentNotFound
			}
			if s.Code() == codes.Unavailable || s.Code() == codes.DeadlineExceeded {
				return "", usecase.ErrPaymentUnavailable
			}
		}
		return "", usecase.ErrPaymentUnavailable
	}

	return resp.Status, nil
}

// Close closes the gRPC connection
func (p *PaymentClient) Close() error {
	return p.conn.Close()
}
