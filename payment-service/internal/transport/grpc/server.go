package grpc

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ap2/payment-service/internal/usecase"

	paymentv1 "ap2/payment-service/internal/transport/grpc/proto/paymentv1"
)

// Server implements the PaymentService gRPC interface
type Server struct {
	paymentv1.UnimplementedPaymentServiceServer
	authorizeUseCase  *usecase.AuthorizePayment
	getPaymentUseCase *usecase.GetPaymentByOrder
}

// NewServer creates a new gRPC server
func NewServer(
	authorizeUseCase *usecase.AuthorizePayment,
	getPaymentUseCase *usecase.GetPaymentByOrder,
) *Server {
	return &Server{
		authorizeUseCase:  authorizeUseCase,
		getPaymentUseCase: getPaymentUseCase,
	}
}

// AuthorizePayment implements the gRPC method for payment authorization
func (s *Server) AuthorizePayment(ctx context.Context, req *paymentv1.AuthorizePaymentRequest) (*paymentv1.AuthorizePaymentResponse, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be greater than zero")
	}

	input := usecase.AuthorizePaymentInput{
		OrderID:       req.OrderId,
		Amount:        req.Amount,
		CustomerEmail: req.GetCustomerEmail(),
	}

	output, err := s.authorizeUseCase.Execute(ctx, input)
	if err != nil {
		if strings.Contains(err.Error(), "customer_email is required") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to authorize payment: %v", err))
	}

	return &paymentv1.AuthorizePaymentResponse{
		PaymentId:     output.PaymentID,
		TransactionId: output.TransactionID,
		Status:        output.Status,
		Amount:        output.Amount,
		CreatedAt:     timestamppb.New(output.CreatedAt),
	}, nil
}

// GetPaymentStatus implements the gRPC method for getting payment status
func (s *Server) GetPaymentStatus(ctx context.Context, req *paymentv1.GetPaymentStatusRequest) (*paymentv1.GetPaymentStatusResponse, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	payment, err := s.getPaymentUseCase.Execute(ctx, req.OrderId)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("payment not found for order_id: %s", req.OrderId))
	}

	return &paymentv1.GetPaymentStatusResponse{
		PaymentId:     payment.ID,
		TransactionId: payment.TransactionID,
		Status:        payment.Status,
		Amount:        payment.Amount,
		CreatedAt:     timestamppb.New(payment.CreatedAt),
	}, nil
}

// RegisterServer registers the payment service with the gRPC server
func RegisterServer(grpcServer *grpc.Server, server *Server) {
	paymentv1.RegisterPaymentServiceServer(grpcServer, server)
}
