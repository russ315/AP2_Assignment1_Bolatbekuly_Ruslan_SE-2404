package http

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.Engine, h *Handlers) {
	r.POST("/payments", h.CreatePayment)
	r.GET("/payments/:order_id", h.GetPaymentByOrderID)
}
