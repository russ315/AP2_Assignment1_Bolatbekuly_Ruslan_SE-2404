package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	orderv1 "stream-client/proto/orderv1"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run stream_client.go <order_id>")
	}

	orderID := os.Args[1]
	orderGrpcAddr := os.Getenv("ORDER_GRPC_ADDR")
	if orderGrpcAddr == "" {
		orderGrpcAddr = "localhost:50052"
	}

	// Connect to order service
	conn, err := grpc.Dial(orderGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := orderv1.NewOrderServiceClient(conn)

	// Create request
	req := &orderv1.OrderRequest{
		OrderId: orderID,
	}

	// Create stream
	stream, err := client.SubscribeToOrderUpdates(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	log.Printf("Subscribed to order updates for: %s", orderID)
	log.Println("Waiting for updates... Press Ctrl+C to exit")

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("\nReceived shutdown signal, exiting...")
		cancel()
	}()

	// Receive updates
	for {
		select {
		case <-ctx.Done():
			return
		default:
			update, err := stream.Recv()
			if err != nil {
				log.Printf("Stream error: %v", err)
				return
			}

			fmt.Printf("\n=== ORDER UPDATE ===\n")
			fmt.Printf("Order ID: %s\n", update.OrderId)
			fmt.Printf("Customer: %s\n", update.CustomerId)
			fmt.Printf("Item: %s\n", update.ItemName)
			fmt.Printf("Amount: %d cents\n", update.Amount)
			fmt.Printf("Status: %s\n", update.Status)
			fmt.Printf("Updated: %s\n", update.UpdatedAt.AsTime().Format(time.RFC3339))
			fmt.Printf("==================\n\n")
		}
	}
}
