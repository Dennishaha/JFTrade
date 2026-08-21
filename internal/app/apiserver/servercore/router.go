package servercore

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apiassistant "github.com/jftrade/jftrade-main/internal/api/assistant"
	apibacktest "github.com/jftrade/jftrade-main/internal/api/backtest"
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	apimd "github.com/jftrade/jftrade-main/internal/api/marketdata"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	apiproducts "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	apiresearch "github.com/jftrade/jftrade-main/internal/api/research"
	apiset "github.com/jftrade/jftrade-main/internal/api/settings"
	apistrat "github.com/jftrade/jftrade-main/internal/api/strategy"
	apiroutes "github.com/jftrade/jftrade-main/internal/api/system"
	apitrading "github.com/jftrade/jftrade-main/internal/api/trading"
	apiwatchlist "github.com/jftrade/jftrade-main/internal/api/watchlist"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/databaseguard"
)

// buildRouter is the single route-assembly list for the console API. Domain
// routes register themselves through their own internal/api package, so this
// function stays a flat wiring sequence: it must not grow per-domain *Server
// methods, because those consume the servercore method budget without adding
// behaviour (see scripts/servercore-budget.json).
func (s *Server) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(requestObservabilityMiddleware(s.observability))
	router.Use(gin.Recovery())
	router.Use(middleware.CORS(s.auth))
	router.Use(s.desktopTokenMiddleware())
	router.Use(s.webAccessMiddleware())
	router.Use(middleware.Auth(s.auth, s.auth, s, s.auth))
	router.Use(func(c *gin.Context) { s.rehearsalProxy(c) })

	router.GET("/swagger", handleSwaggerRoot)
	router.GET("/swagger/*any", handleSwaggerUI)

	api := router.Group("/api/v1")
	databaseRoutes := databaseguard.New(api, s.unavailableDatabases)

	auth := api.Group("/auth")
	auth.POST("/login", s.auth.Login)
	auth.POST("/logout", s.auth.Logout)
	auth.GET("/session", s.auth.Status)

	api.GET("/ws/live", gin.WrapH(liveHandlerOrNotFound(s.runtimes.LiveWebSocket())))

	apimd.RegisterRoutes(api, s.marketdataSvc, s.productFeaturesSvc)
	apiproducts.RegisterRoutes(api, s.productFeaturesSvc)
	apiresearch.RegisterRoutes(databaseRoutes.Research, s.researchSvc)
	apiset.RegisterRoutes(api, s.settingsSvc, s.dataManagementSvc)
	apiroutes.RegisterRoutes(api, s.sysSvc)
	s.registerResource("assistant HTTP transport", apiassistant.RegisterRoutes(databaseRoutes.Assistant, s.assistantSvc).Close)
	apistrat.RegisterPluginRoutes(databaseRoutes.Strategy, s.strategySvc)
	apistrat.RegisterRoutes(databaseRoutes.Strategy, s.strategySvc)
	apibacktest.RegisterRoutes(databaseRoutes.Backtest, s.backtestSvc)
	apitrading.RegisterRoutes(api, s.tradingSvc)
	apitrading.RegisterPortfolioRoutes(api, s.tradingSvc)
	apitrading.RegisterExecutionRoutes(databaseRoutes.Execution, s.tradingSvc)
	apiwatchlist.RegisterRoutes(databaseRoutes.Watchlist, s.watchlistSvc)

	router.NoRoute(s.handleNoRoute)
	return router
}

func liveHandlerOrNotFound(handler *apilive.Handler) http.Handler {
	if handler == nil {
		return http.NotFoundHandler()
	}
	return handler
}

func (s *Server) handleNoRoute(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		notFound(s, c)
		return
	}
	if s.frontend != nil && s.frontend.serveRequest(c.Writer, c.Request) {
		return
	}
	notFound(s, c)
}

func notFound(s *Server, c *gin.Context) {
	writeError(s, c, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("unknown endpoint %s", c.Request.URL.Path))
}
