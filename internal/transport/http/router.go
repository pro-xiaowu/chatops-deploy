package http

import (
	"context"
	stdhttp "net/http"

	"chatops-deploy/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Readiness func(context.Context) error
}

func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	health := handler.NewHealth(deps.Readiness)
	router.GET("/healthz", func(c *gin.Context) {
		respondData(c, stdhttp.StatusOK, health.Liveness())
	})
	router.GET("/readyz", func(c *gin.Context) {
		status, err := health.Readiness(c.Request.Context())
		if err != nil {
			respondError(c, stdhttp.StatusServiceUnavailable, "service_unavailable", "service is not ready")
			return
		}
		respondData(c, stdhttp.StatusOK, status)
	})

	return router
}
