package http

type createOrderRequest struct {
	CustomerID     string `json:"customer_id" binding:"required"`
	CustomerEmail  string `json:"customer_email" binding:"required"`
	ItemName       string `json:"item_name" binding:"required"`
	Amount         int64  `json:"amount" binding:"required,gt=0"`
}
