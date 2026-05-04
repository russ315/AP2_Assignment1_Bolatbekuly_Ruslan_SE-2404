package http

type createPaymentRequest struct {
	OrderID       string `json:"order_id" binding:"required"`
	Amount        int64  `json:"amount" binding:"required,gt=0"`
	CustomerEmail string `json:"customer_email" binding:"required"`
}

type createPaymentResponse struct {
	PaymentID       string `json:"payment_id"`
	TransactionID   string `json:"transaction_id"`
	Status          string `json:"status"`
	Amount          int64  `json:"amount"`
	CreatedAt       string `json:"created_at"`
}

type getPaymentResponse struct {
	PaymentID       string `json:"payment_id"`
	OrderID         string `json:"order_id"`
	TransactionID   string `json:"transaction_id"`
	Amount          int64  `json:"amount"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
}
