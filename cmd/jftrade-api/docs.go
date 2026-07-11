package main

//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g docs.go -d .,../../internal/app/apiserver/servercore,../../internal/api/system,../../internal/api/marketdata,../../internal/api/assistant,../../internal/api/backtest,../../internal/api/settings,../../internal/api/strategy,../../internal/api/trading,../../internal/api/watchlist -o ../../docs/swagger --parseDependency --parseInternal

// @title JFTrade Debug API
// @version 1.0.0
// @description JFTrade sidecar API 的调试文档。Swagger UI 主要覆盖当前常用的 HTTP 调试入口，并展示实时接口的请求方式。
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey AdministratorBearer
// @in header
// @name Authorization
// @description Bearer <JFTrade administrator key>
// @securityDefinitions.apikey AdministratorSession
// @in cookie
// @name jftrade_admin_session
