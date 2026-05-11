package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ap2/payment-service/internal/usecase"
)

type Handlers struct {
	authorize *usecase.AuthorizePayment
	getByOrder *usecase.GetPaymentByOrder
}

func NewHandlers(authorize *usecase.AuthorizePayment, getByOrder *usecase.GetPaymentByOrder) *Handlers {
	return &Handlers{authorize: authorize, getByOrder: getByOrder}
}

func (h *Handlers) CreatePayment(c *gin.Context) {
	var req createPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	out, err := h.authorize.Execute(c.Request.Context(), usecase.AuthorizePaymentInput{
		OrderID:       strings.TrimSpace(req.OrderID),
		Amount:        req.Amount,
		CustomerEmail: strings.TrimSpace(req.CustomerEmail),
	})
	if err != nil {
		if strings.Contains(err.Error(), "amount must be greater than zero") ||
			strings.Contains(err.Error(), "order_id is required") ||
			strings.Contains(err.Error(), "customer_email is required") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if strings.Contains(err.Error(), "payment already exists") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, createPaymentResponse{
		PaymentID:     out.PaymentID,
		TransactionID: out.TransactionID,
		Status:        out.Status,
		Amount:        out.Amount,
		CreatedAt:     out.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *Handlers) GetPaymentByOrderID(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("order_id"))
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_id is required"})
		return
	}

	p, err := h.getByOrder.Execute(c.Request.Context(), orderID)
	if err != nil {
		if strings.Contains(err.Error(), "order_id is required") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
		return
	}

	c.JSON(http.StatusOK, getPaymentResponse{
		PaymentID:     p.ID,
		OrderID:       p.OrderID,
		TransactionID: p.TransactionID,
		Amount:        p.Amount,
		Status:        p.Status,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
