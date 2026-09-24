package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// SetAsyncMediaRouter registers the explicit asynchronous media job surface.
// Acceptance happens on the OpenAI image endpoints themselves (`?async=true`,
// wired by AsyncMediaEnqueue); these routes only read the accepted job back.
//
// The namespace is host-owned and disjoint from the generic `/v1/tasks`
// surface, which stays the plugin task API.
func SetAsyncMediaRouter(router *gin.Engine) {
	jobRouter := router.Group("/v1/async")
	jobRouter.Use(middleware.RouteTag("relay"), middleware.TokenAuthReadOnly())
	{
		jobRouter.GET("/tasks/:job_id", controller.GetAsyncMediaJob)
	}
}
