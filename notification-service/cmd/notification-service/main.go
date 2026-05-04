package main

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"

	"ap2/notification-service/internal/app"
	"ap2/notification-service/internal/postgres"
)

//go:embed migration.sql
var migrationSQL string

const (
	exchangeName       = "notifications.events"
	exchangeKind       = "topic"
	routingKey         = "payment.completed"
	queueName          = "payment.completed"
	dlxExchangeName    = "notifications.dlx"
	dlqQueueName       = "payment.completed.dlq"
	dlxRoutingKey      = "payment.dead"
)

func main() {
	dsn := os.Getenv("NOTIFICATION_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/notification_db?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := db.PingContext(ctx); err != nil {
		cancel()
		log.Fatalf("db ping: %v", err)
	}
	cancel()

	if _, err := db.Exec(migrationSQL); err != nil {
		log.Fatalf("migration: %v", err)
	}

	store := postgres.NewStore(db)

	amqpURL := os.Getenv("NOTIFICATION_RABBITMQ_URL")
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@localhost:5672/"
	}
	dlqDemo := os.Getenv("NOTIFICATION_DLQ_DEMO_ORDER_ID")

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		log.Fatalf("rabbitmq dial: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("rabbitmq channel: %v", err)
	}
	defer ch.Close()

	if err := ch.Qos(1, 0, false); err != nil {
		log.Fatalf("rabbitmq qos: %v", err)
	}

	if err := ch.ExchangeDeclare(
		dlxExchangeName, "direct", true, false, false, false, nil,
	); err != nil {
		log.Fatalf("dlx exchange: %v", err)
	}
	if _, err := ch.QueueDeclare(
		dlqQueueName, true, false, false, false, nil,
	); err != nil {
		log.Fatalf("dlq queue: %v", err)
	}
	if err := ch.QueueBind(dlqQueueName, dlxRoutingKey, dlxExchangeName, false, nil); err != nil {
		log.Fatalf("dlq bind: %v", err)
	}

	if err := ch.ExchangeDeclare(
		exchangeName, exchangeKind, true, false, false, false, nil,
	); err != nil {
		log.Fatalf("events exchange: %v", err)
	}

	qArgs := amqp.Table{
		"x-dead-letter-exchange":    dlxExchangeName,
		"x-dead-letter-routing-key": dlxRoutingKey,
	}
	if _, err := ch.QueueDeclare(
		queueName, true, false, false, false, qArgs,
	); err != nil {
		log.Fatalf("main queue: %v", err)
	}
	if err := ch.QueueBind(queueName, routingKey, exchangeName, false, nil); err != nil {
		log.Fatalf("queue bind: %v", err)
	}

	const consumerTag = "notification-service"

	msgs, err := ch.Consume(
		queueName,
		consumerTag,
		false, // manual ack
		false, false, false, nil,
	)
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutdown signal received, stopping consumer...")
		if err := ch.Cancel(consumerTag, false); err != nil {
			log.Printf("consumer cancel: %v", err)
		}
		workerCancel()
	}()

	log.Printf("notification-service consuming queue=%s (manual ack, durable)", queueName)

	for {
		select {
		case <-workerCtx.Done():
			log.Println("consumer stopped")
			return
		case d, ok := <-msgs:
			if !ok {
				return
			}
			handle(workerCtx, ch, d, store, dlqDemo)
		}
	}
}

func handle(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, store *postgres.Store, dlqDemo string) {
	procCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := app.Process(procCtx, store, dlqDemo, d.Body)
	if err == nil {
		if err := d.Ack(false); err != nil {
			log.Printf("ack: %v", err)
		}
		return
	}

	switch {
	case errors.Is(err, app.ErrPoison), errors.Is(err, app.ErrForceDLQDemo):
		log.Printf("nack (dead-letter): %v", err)
		if err := d.Nack(false, false); err != nil {
			log.Printf("nack: %v", err)
		}
	default:
		log.Printf("transient failure, requeue: %v", err)
		if err := d.Nack(false, true); err != nil {
			log.Printf("nack requeue: %v", err)
		}
	}
}
