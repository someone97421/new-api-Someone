package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetFileRelayRouter(router *gin.Engine) {
	files := router.Group("/v1/file-relay", middleware.RouteTag("relay"))
	files.POST("", middleware.TokenAuth(), controller.CreateFileRelay)
	files.GET("/content/:id", controller.FileRelayContent)
	files.HEAD("/content/:id", controller.FileRelayContent)
}
