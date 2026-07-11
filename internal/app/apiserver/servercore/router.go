package servercore

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apiassistant "github.com/jftrade/jftrade-main/internal/api/assistant"
	apibacktest "github.com/jftrade/jftrade-main/internal/api/backtest"
	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	apimd "github.com/jftrade/jftrade-main/internal/api/marketdata"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	apiset "github.com/jftrade/jftrade-main/internal/api/settings"
	apistrat "github.com/jftrade/jftrade-main/internal/api/strategy"
	apiroutes "github.com/jftrade/jftrade-main/internal/api/system"
	apitrading "github.com/jftrade/jftrade-main/internal/api/trading"
	apiwatchlist "github.com/jftrade/jftrade-main/internal/api/watchlist"
)

func (s *Server) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(requestObservabilityMiddleware(s.observability))
	router.Use(gin.Recovery())
	router.Use(s.corsMiddleware())
	router.Use(s.desktopTokenMiddleware())
	router.Use(s.authMiddleware())
	router.Use(s.databaseAvailabilityMiddleware())

	router.GET("/swagger", s.handleSwaggerRoot)
	router.GET("/swagger/*any", s.handleSwaggerUI)

	api := router.Group("/api/v1")
	s.registerAuthRoutes(api)
	s.registerMarketRoutes(api)
	s.registerSettingsRoutes(api)
	s.registerSystemRoutes(api)
	s.registerADKRoutes(api)
	s.registerPluginRoutes(api)
	s.registerStrategyRoutes(api)
	s.registerBacktestRoutes(api)
	s.registerBrokerRoutes(api)
	s.registerPortfolioRoutes(api)
	s.registerExecutionRoutes(api)
	s.registerWatchlistRoutes(api)

	router.NoRoute(s.handleNoRoute)
	return router
}

func (s *Server) databaseAvailabilityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		required := []string{}
		switch {
		case strings.HasPrefix(path, "/api/v1/backtests"):
			required = []string{"backtest", "backtest-runs"}
		case strings.HasPrefix(path, "/api/v1/strategy"), strings.HasPrefix(path, "/api/v1/strategies"):
			required = []string{"strategy"}
		case strings.HasPrefix(path, "/api/v1/execution"):
			required = []string{"execution-orders"}
		case strings.HasPrefix(path, "/api/v1/adk"), strings.HasPrefix(path, "/api/v1/assistant"):
			required = []string{"adk", "adk-session"}
		case strings.HasPrefix(path, "/api/v1/watchlist"):
			required = []string{"watchlist"}
		}
		for _, id := range required {
			if err := s.unavailableDatabases[id]; err != nil {
				httpserver.WriteError(c, http.StatusServiceUnavailable, "DATABASE_INCOMPATIBLE", fmt.Sprintf("%s database is unavailable; rebuild it in Settings > 数据库重建 and restart JFTrade", id))
				return
			}
		}
		c.Next()
	}
}

func (s *Server) registerWatchlistRoutes(api *gin.RouterGroup) {
	apiwatchlist.RegisterRoutes(api, s.watchlistSvc)
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return middleware.CORS(s.auth)
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return middleware.Auth(s.auth, s.auth, s, s.auth)
}

func (s *Server) registerAuthRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	auth.POST("/login", s.auth.login)
	auth.POST("/logout", s.handleAuthLogout)
	auth.GET("/session", s.auth.status)
	auth.Any("/token", s.handleAuthTokenDeprecated)
}

// registerMarketRoutes godoc
// @Summary 实时行情 WebSocket
// @Tags market-data
// @Produce json
// @Success 101 {string} string "Switching Protocols"
// @Router /api/v1/ws/live [get]
func (s *Server) registerMarketRoutes(api *gin.RouterGroup) {
	api.GET("/ws/live", gin.WrapH(liveHandlerOrNotFound(s.liveWebSocket)))

	// HTTP market data — 委托到 marketdata Service
	apimd.RegisterRoutes(api, s.marketdataSvc)
}

func liveHandlerOrNotFound(handler *apilive.Handler) http.Handler {
	if handler == nil {
		return http.NotFoundHandler()
	}
	return handler
}

func (s *Server) registerSettingsRoutes(api *gin.RouterGroup) {
	apiset.RegisterRoutes(api, s.settingsSvc, s.dataManagementSvc)
	api.POST("/settings/system-notifications/test", s.handleSystemNotificationTest)
}

// handleSystemNotificationTest godoc
// @Summary 发送系统通知测试事件
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 500 {object} httpserver.Envelope
// @Router /api/v1/settings/system-notifications/test [post]
func (s *Server) handleSystemNotificationTest(c *gin.Context) {
	event, delivery := s.recordLiveNotificationWithDelivery(liveNotification{
		Level:    "warn",
		Title:    "JFTrade 系统通知测试",
		Message:  "系统通知通道已连接。",
		Source:   "desktop",
		Category: "system.notification.test",
	})
	if event == nil {
		httpserver.WriteError(c, 500, "SYSTEM_NOTIFICATION_TEST_FAILED", "notification publisher is unavailable")
		return
	}
	httpserver.WriteOK(c, map[string]any{
		"event":    liveNotificationEventMap(*event),
		"delivery": delivery,
	})
}

func (s *Server) registerSystemRoutes(api *gin.RouterGroup) {
	apiroutes.RegisterRoutes(api, s.sysSvc)
}

func (s *Server) registerADKRoutes(api *gin.RouterGroup) {
	apiassistant.RegisterRoutes(api, s.assistantSvc)
}

func (s *Server) registerPluginRoutes(api *gin.RouterGroup) {
	apistrat.RegisterPluginRoutes(api, s.strategySvc)
}

func (s *Server) registerStrategyRoutes(api *gin.RouterGroup) {
	apistrat.RegisterRoutes(api, s.strategySvc)
}

func (s *Server) registerBacktestRoutes(api *gin.RouterGroup) {
	apibacktest.RegisterRoutes(api, s.backtestSvc)
}

func (s *Server) registerBrokerRoutes(api *gin.RouterGroup) {
	apitrading.RegisterRoutes(api, s.tradingSvc)
}

func (s *Server) registerPortfolioRoutes(api *gin.RouterGroup) {
	apitrading.RegisterPortfolioRoutes(api, s.tradingSvc)
}

func (s *Server) registerExecutionRoutes(api *gin.RouterGroup) {
	apitrading.RegisterExecutionRoutes(api, s.tradingSvc)
}

func (s *Server) handleAuthLogout(c *gin.Context) {
	if !s.authorizeRequest(c) {
		return
	}
	s.auth.logout(c)
}

// handleAuthTokenDeprecated godoc
// @Summary 旧令牌入口（已退役）
// @Description 始终返回 410 Gone；CLI 请直接使用管理员密钥作为 Bearer token。
// @Tags auth
// @Produce json
// @Failure 410 {object} envelope
// @Router /api/v1/auth/token [get]
func (s *Server) handleAuthTokenDeprecated(c *gin.Context) {
	s.writeError(c, http.StatusGone, "AUTH_TOKEN_REMOVED", "use administrator login or a Bearer administrator key")
}

func (s *Server) handleNoRoute(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		s.notFound(c)
		return
	}
	if s.frontend != nil && s.frontend.serveRequest(c.Writer, c.Request) {
		return
	}
	s.notFound(c)
}

func (s *Server) notFound(c *gin.Context) {
	s.writeError(c, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("unknown endpoint %s", c.Request.URL.Path))
}
