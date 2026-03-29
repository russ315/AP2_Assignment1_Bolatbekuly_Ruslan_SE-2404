package http

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.Engine, h *Handlers) {
	r.POST("/orders", h.CreateOrder)
	r.GET("/orders/:id", h.GetOrder)
	r.PATCH("/orders/:id/cancel", h.CancelOrder)
}
