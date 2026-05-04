package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"ap2/payment-service/internal/usecase"
)

const (
	exchangeName            = "notifications.events"
	exchangeKind            = "topic"
	routingPaymentCompleted = "payment.completed"
)

// Publisher publishes payment.completed events with publisher confirms enabled.
type Publisher struct {
	conn     *amqp.Connection
	ch       *amqp.Channel
	confirms <-chan amqp.Confirmation
}

// DialPublisher connects to RabbitMQ and declares the topic exchange.
func DialPublisher(amqpURL string) (*Publisher, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}f
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq confirm mode: %w", err)
	}
	confirms := make(chan amqp.Confirmation, 8)
	ch.NotifyPublish(confirms)

	if err := ch.ExchangeDeclare(
		exchangeName,
		exchangeKind,
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq exchange declare: %w", err)
	}
	return &Publisher{conn: conn, ch: ch, confirms: confirms}, nil
}

type paymentCompletedPayload struct {
	EventID       string `json:"event_id"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	CustomerEmail string `json:"customer_email"`
	Status        string `json:"status"`
}

// PublishPaymentCompleted implements usecase.PaymentCompletedPublisher.
func (p *Publisher) PublishPaymentCompleted(ctx context.Context, e usecase.PaymentCompletedEvent) error {
	body, err := json.Marshal(paymentCompletedPayload{
		EventID:       e.EventID,
		OrderID:       e.OrderID,
		Amount:        e.AmountCents,
		CustomerEmail: e.CustomerEmail,
		Status:        e.Status,
	})
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	err = p.ch.PublishWithContext(ctx,
		exchangeName,
		routingPaymentCompleted,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c, ok := <-p.confirms:
		if !ok {
			return fmt.Errorf("publisher confirms channel closed")
		}
		if !c.Ack {
			return fmt.Errorf("broker did not ack publish")
		}
	}
	return nil
}

// Close releases the channel and connection.
func (p *Publisher) Close() error {
	if p == nil {
		return nil
	}
	var err error
	if p.ch != nil {
		err = p.ch.Close()
	}
	if p.conn != nil {
		if cerr := p.conn.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}
