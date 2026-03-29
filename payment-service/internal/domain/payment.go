package domain

import "time"

// Payment represents a payment authorization attempt in the Payment bounded context.
// Amount is stored in cents (int64); never use float for money.
type Payment struct {
	ID            string
	OrderID       string
	TransactionID string
	Amount        int64
	Status        string
	CreatedAt     time.Time
}

const (
	StatusAuthorized = "Authorized"
	StatusDeclined   = "Declined"
)

// MaxAuthorizedAmountCents is the inclusive ceiling for automatic authorization.
// Amounts strictly greater than this value are declined.
const MaxAuthorizedAmountCents int64 = 100000
