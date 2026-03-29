package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	paymentadapter "ap2/order-service/internal/adapter/payment"
	"ap2/order-service/internal/repository/postgres"
	httpx "ap2/order-service/internal/transport/http"
	"ap2/order-service/internal/usecase"
)

func main() {
	dsn := os.Getenv("ORDER_DATABASE_URL")
	if dsn == "" {
		dsn = "<defaultur>"
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

	paymentBaseURL := os.Getenv("PAYMENT_SERVICE_URL")
	if paymentBaseURL == "" {
		paymentBaseURL = "http://localhost:8081"
	}

	// Shared outbound client with explicit timeout (assignment: max 2 seconds).
	paymentHTTP := &http.Client{
		Timeout: 2 * time.Second,
	}
	paymentClient := paymentadapter.NewRestClient(paymentBaseURL, paymentHTTP)

	orderRepo := postgres.NewOrderRepository(db)
	createUC := usecase.NewCreateOrder(orderRepo, paymentClient)
	getUC := usecase.NewGetOrder(orderRepo)
	cancelUC := usecase.NewCancelOrder(orderRepo)
	h := httpx.NewHandlers(createUC, getUC, cancelUC)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	httpx.RegisterRoutes(r, h)

	addr := os.Getenv("ORDER_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("order-service listening on %s (payment API %s)", addr, paymentBaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown: %v\n", err)
	}
}
