package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
)

// LoggingInterceptor logs every incoming gRPC request
func LoggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	
	log.Printf("gRPC call started - Method: %s", info.FullMethod)
	
	resp, err := handler(ctx, req)
	
	duration := time.Since(start)
	if err != nil {
		log.Printf("gRPC call failed - Method: %s, Duration: %v, Error: %v", info.FullMethod, duration, err)
	} else {
		log.Printf("gRPC call completed - Method: %s, Duration: %v", info.FullMethod, duration)
	}
	
	return resp, err
}

// StreamLoggingInterceptor logs streaming gRPC requests
func StreamLoggingInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	
	log.Printf("gRPC stream started - Method: %s", info.FullMethod)
	
	err := handler(srv, ss)
	
	duration := time.Since(start)
	if err != nil {
		log.Printf("gRPC stream failed - Method: %s, Duration: %v, Error: %v", info.FullMethod, duration, err)
	} else {
		log.Printf("gRPC stream completed - Method: %s, Duration: %v", info.FullMethod, duration)
	}
	
	return err
}
