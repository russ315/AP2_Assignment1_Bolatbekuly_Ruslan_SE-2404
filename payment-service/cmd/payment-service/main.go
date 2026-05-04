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

	"ap2/payment-service/internal/repository/postgres"
	"ap2/payment-service/internal/infrastructure/rabbitmq"
	grpcx "ap2/payment-service/internal/transport/grpc"
	httpx "ap2/payment-service/internal/transport/http"
	"ap2/payment-service/internal/usecase"
)

func main() {
	dsn := os.Getenv("PAYMENT_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://postgres:Ruslan2006%40@localhost:5432/payment_db?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Minute * 5)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	repo := postgres.NewPaymentRepository(db)

	var publisher usecase.PaymentCompletedPublisher
	amqpURL := os.Getenv("PAYMENT_RABBITMQ_URL")
	if amqpURL != "" {
		pub, err := rabbitmq.DialPublisher(amqpURL)
		if err != nil {
			log.Fatalf("rabbitmq publisher: %v", err)
		}
		defer func() {
			if err := pub.Close(); err != nil {
				log.Printf("rabbitmq close: %v", err)
			}
		}()
		publisher = pub
	}

	authorizeUC := usecase.NewAuthorizePayment(repo, publisher)
	getUC := usecase.NewGetPaymentByOrder(repo)
	h := httpx.NewHandlers(authorizeUC, getUC)

	// Setup gRPC server
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcx.LoggingInterceptor),
		grpc.StreamInterceptor(grpcx.StreamLoggingInterceptor),
	)
	paymentGrpcServer := grpcx.NewServer(authorizeUC, getUC)
	grpcx.RegisterServer(grpcServer, paymentGrpcServer)

	grpcAddr := os.Getenv("PAYMENT_GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":50051"
	}

	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on gRPC port %s: %v", grpcAddr, err)
	}

	go func() {
		log.Printf("payment-service gRPC listening on %s", grpcAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Setup HTTP server
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	httpx.RegisterRoutes(r, h)

	httpAddr := os.Getenv("PAYMENT_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8081"
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
		log.Printf("payment-service HTTP listening on %s", httpAddr)
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

	grpcServer.GracefulStop()
	log.Println("Servers stopped")
}
