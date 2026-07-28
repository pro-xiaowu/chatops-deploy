package http

import (
	"context"
	stdhttp "net/http"
	"os"

	"chatops-deploy/internal/auth"
	"chatops-deploy/internal/transport/http/handler"
	"chatops-deploy/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Dependencies struct {
	Readiness func(context.Context) error
	API       *handler.API
	Tokens    *auth.Service
}

func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())

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
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	if deps.API != nil {
		router.GET("/auth/capabilities", deps.API.Capabilities)
		router.POST("/auth/dev/login", deps.API.DevLogin)
		router.POST("/webhooks/:provider", deps.API.Webhook)
		router.GET("/auth/feishu/login", deps.API.Login)
		router.GET("/auth/feishu/callback", deps.API.OAuthCallback)
		if deps.Tokens != nil {
			api := router.Group("/api/v1", middleware.RequireAuth(deps.Tokens))
			deps.API.Register(api)
			router.GET("/events", middleware.RequireAuth(deps.Tokens), deps.API.Events)
		}
	}
	if _, err := os.Stat("web/dist/index.html"); err == nil {
		router.Static("/assets", "web/dist/assets")
		router.NoRoute(func(c *gin.Context) { c.File("web/dist/index.html") })
	}

	return router
}
