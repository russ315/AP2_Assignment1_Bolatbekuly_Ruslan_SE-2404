package email

import "context"

// SendInput is a vendor-neutral payment notification email.
type SendInput struct {
	ToEmail     string
	OrderID     string
	PaymentID   string
	AmountCents int64
	Status      string
}

// Sender sends notification emails (Mailjet, SMTP, simulated, etc.).
type Sender interface {
	SendPaymentReceipt(ctx context.Context, in SendInput) error
}
