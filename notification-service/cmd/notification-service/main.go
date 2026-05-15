package main

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"ap2/notification-service/internal/email"
	"ap2/notification-service/internal/notify"
	"ap2/notification-service/internal/postgres"
	"ap2/notification-service/internal/redisnotify"
)

//go:embed migration.sql
var migrationSQL string

const (
	exchangeName    = "notifications.events"
	exchangeKind    = "topic"
	routingKey      = "payment.completed"
	queueName       = "payment.completed"
	dlxExchangeName = "notifications.dlx"
	dlqQueueName    = "payment.completed.dlq"
	dlxRoutingKey   = "payment.dead"
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

	pgStore := postgres.NewStore(db)

	redisAddr := strings.TrimSpace(os.Getenv("NOTIFICATION_REDIS_ADDR"))
	if redisAddr == "" {
		log.Fatal("NOTIFICATION_REDIS_ADDR is required (Redis idempotency)")
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(rctx).Err(); err != nil {
		rcancel()
		log.Fatalf("redis ping: %v", err)
	}
	rcancel()

	idemTTL := 7 * 24 * time.Hour
	if v := strings.TrimSpace(os.Getenv("NOTIFICATION_IDEMPOTENCY_TTL_HOURS")); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			idemTTL = time.Duration(h) * time.Hour
		}
	}
	redisStore := redisnotify.New(rdb, idemTTL)

	sender := buildEmailSender()

	maxAttempts := 4
	if v := strings.TrimSpace(os.Getenv("NOTIFICATION_EMAIL_MAX_ATTEMPTS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxAttempts = n
		}
	}
	backoffBase := 2 * time.Second
	if v := strings.TrimSpace(os.Getenv("NOTIFICATION_BACKOFF_BASE_MS")); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			backoffBase = time.Duration(ms) * time.Millisecond
		}
	}

	proc := &notify.Processor{
		PG:             pgStore,
		Redis:          redisStore,
		Sender:         sender,
		DLQDemoOrderID: strings.TrimSpace(os.Getenv("NOTIFICATION_DLQ_DEMO_ORDER_ID")),
		MaxAttempts:    maxAttempts,
		BackoffBase:    backoffBase,
	}

	amqpURL := os.Getenv("NOTIFICATION_RABBITMQ_URL")
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@localhost:5672/"
	}

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

	mode := strings.ToUpper(strings.TrimSpace(os.Getenv("PROVIDER_MODE")))
	if mode == "" {
		mode = "SIMULATED"
	}
	log.Printf("notification-service queue=%s manual_ack=true provider_mode=%s redis_idempotency=true max_attempts=%d backoff_base=%s",
		queueName, mode, maxAttempts, backoffBase)

	for {
		select {
		case <-workerCtx.Done():
			log.Println("consumer stopped")
			return
		case d, ok := <-msgs:
			if !ok {
				return
			}
			handle(workerCtx, ch, d, proc)
		}
	}
}

func buildEmailSender() email.Sender {
	mode := strings.ToUpper(strings.TrimSpace(os.Getenv("PROVIDER_MODE")))
	if mode == "" {
		mode = "SIMULATED"
	}
	if mode == "REAL" {
		key := strings.TrimSpace(os.Getenv("MAILJET_API_KEY"))
		secret := strings.TrimSpace(os.Getenv("MAILJET_API_SECRET"))
		from := strings.TrimSpace(os.Getenv("MAILJET_FROM_EMAIL"))
		name := strings.TrimSpace(os.Getenv("MAILJET_FROM_NAME"))
		log.Println("email provider: Mailjet (REAL)")
		return email.NewMailjet(key, secret, from, name)
	}
	minLat := 50 * time.Millisecond
	maxLat := 200 * time.Millisecond
	if v := strings.TrimSpace(os.Getenv("NOTIFICATION_SIM_MIN_LATENCY_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			minLat = time.Duration(n) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("NOTIFICATION_SIM_MAX_LATENCY_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxLat = time.Duration(n) * time.Millisecond
		}
	}
	failRate := 0.2
	if v := strings.TrimSpace(os.Getenv("NOTIFICATION_SIM_FAIL_RATE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			failRate = f
		}
	}
	log.Printf("email provider: SIMULATED min=%s max=%s fail_rate=%.2f", minLat, maxLat, failRate)
	return email.NewSimulated(minLat, maxLat, failRate)
}

func handle(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, proc *notify.Processor) {
	procCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	err := proc.Handle(procCtx, d.Body)
	if err == nil {
		if err := d.Ack(false); err != nil {
			log.Printf("ack: %v", err)
		}
		return
	}

	switch {
	case errors.Is(err, notify.ErrPoison), errors.Is(err, notify.ErrForceDLQDemo):
		log.Printf("nack (dead-letter): %v", err)
		if err := d.Nack(false, false); err != nil {
			log.Printf("nack: %v", err)
		}
	case errors.Is(err, notify.ErrTransient):
		log.Printf("transient failure, requeue: %v", err)
		if err := d.Nack(false, true); err != nil {
			log.Printf("nack requeue: %v", err)
		}
	default:
		log.Printf("transient failure (unknown), requeue: %v", err)
		if err := d.Nack(false, true); err != nil {
			log.Printf("nack requeue: %v", err)
		}
	}
}
