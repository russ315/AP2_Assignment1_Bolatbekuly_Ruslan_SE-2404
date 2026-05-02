package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"

	paymentadapter "ap2/order-service/internal/adapter/grpc"
	"ap2/order-service/internal/repository/postgres"
	grpcx "ap2/order-service/internal/transport/grpc"
	httpx "ap2/order-service/internal/transport/http"
	"ap2/order-service/internal/usecase"
)

func main() {
	dsn := os.Getenv("ORDER_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://postgres:Ruslan2006%40@localhost:5432/order_db?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	paymentGrpcAddr := os.Getenv("PAYMENT_GRPC_ADDR")
	if paymentGrpcAddr == "" {
		paymentGrpcAddr = "localhost:50051"
	}

	paymentClient, err := paymentadapter.NewPaymentClient(paymentGrpcAddr)
	if err != nil {
		log.Fatalf("failed to create payment client: %v", err)
	}
	defer paymentClient.Close()

	orderRepo := postgres.NewOrderRepository(db)
	createUC := usecase.NewCreateOrder(orderRepo, paymentClient)
	getUC := usecase.NewGetOrder(orderRepo)
	cancelUC := usecase.NewCancelOrder(orderRepo)
	h := httpx.NewHandlers(createUC, getUC, cancelUC)

	// Setup gRPC server for order streaming
	grpcServer := grpc.NewServer()
	orderGrpcServer := grpcx.NewOrderServer(db)
	grpcx.RegisterOrderServer(grpcServer, orderGrpcServer)

	orderGrpcAddr := os.Getenv("ORDER_GRPC_ADDR")
	if orderGrpcAddr == "" {
		orderGrpcAddr = ":50052"
	}

	grpcListener, err := net.Listen("tcp", orderGrpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on gRPC port %s: %v", orderGrpcAddr, err)
	}

	go func() {
		log.Printf("order-service gRPC streaming listening on %s", orderGrpcAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Setup HTTP server
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	httpx.RegisterRoutes(r, h)

	httpAddr := os.Getenv("ORDER_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8082"
	}

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("order-service HTTP listening on %s (payment gRPC %s)", httpAddr, paymentGrpcAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	log.Println("Shutting down servers...")

	go func() {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "HTTP shutdown: %v\n", err)
		}
	}()

	orderGrpcServer.Stop()
	grpcServer.GracefulStop()
	log.Println("Servers stopped")
}
