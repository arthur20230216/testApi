package server

import (
	"github.com/gin-gonic/gin"

	"modelprobe/backend/internal/config"
	"modelprobe/backend/internal/handler"
)

func NewRouter(cfg config.Config, probeHandler *handler.ProbeHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), corsMiddleware(cfg.AllowOrigin))

	api := router.Group("/api")
	probeHandler.Register(api)

	return router
}

func corsMiddleware(allowOrigin string) gin.HandlerFunc {
	return func(context *gin.Context) {
		origin := allowOrigin
		if origin == "" {
			origin = "*"
		}

		context.Header("Access-Control-Allow-Origin", origin)
		context.Header("Access-Control-Allow-Credentials", "true")
		context.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		context.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")

		if context.Request.Method == "OPTIONS" {
			context.AbortWithStatus(204)
			return
		}

		context.Next()
	}
}
