package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ap2/order-service/internal/domain"
	"ap2/order-service/internal/usecase"
)

type Handlers struct {
	create *usecase.CreateOrder
	get    *usecase.GetOrder
	cancel *usecase.CancelOrder
}

func NewHandlers(create *usecase.CreateOrder, get *usecase.GetOrder, cancel *usecase.CancelOrder) *Handlers {
	return &Handlers{create: create, get: get, cancel: cancel}
}

func (h *Handlers) CreateOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	var idem *string
	if v := strings.TrimSpace(c.GetHeader("Idempotency-Key")); v != "" {
		idem = &v
	}

	order, err := h.create.Execute(c.Request.Context(), usecase.CreateOrderInput{
		CustomerID:     req.CustomerID,
		CustomerEmail:  req.CustomerEmail,
		ItemName:       req.ItemName,
		Amount:         req.Amount,
		IdempotencyKey: idem,
	})
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "customer_id, customer_email, item_name, and amount > 0 are required"})
			return
		}
		if errors.Is(err, usecase.ErrPaymentUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "payment service unavailable",
				"order": toOrderResponse(order),
			})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
			"order": toOrderResponse(order),
		})
		return
	}

	c.JSON(http.StatusOK, toOrderResponse(order))
}

func (h *Handlers) GetOrder(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	order, err := h.get.Execute(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, usecase.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, toOrderResponse(order))
}

func (h *Handlers) CancelOrder(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	order, err := h.cancel.Execute(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, usecase.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		if errors.Is(err, usecase.ErrCancelNotAllowed) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "only pending orders can be cancelled; Paid, Failed, and Cancelled orders cannot",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, toOrderResponse(order))
}

func toOrderResponse(o *domain.Order) gin.H {
	if o == nil {
		return nil
	}
	return gin.H{
		"id":          o.ID,
		"customer_id": o.CustomerID,
		"item_name":   o.ItemName,
		"amount":      o.Amount,
		"status":      o.Status,
		"created_at":  o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
