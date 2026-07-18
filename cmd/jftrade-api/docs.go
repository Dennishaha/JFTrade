package main

//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g docs.go -d .,../../internal/app/apiserver/servercore,../../internal/api/system,../../internal/api/marketdata,../../internal/api/productfeatures,../../internal/api/assistant,../../internal/api/backtest,../../internal/api/settings,../../internal/api/strategy,../../internal/api/trading,../../internal/api/watchlist -o ../../docs/swagger --parseDependency --parseInternal

// @title JFTrade Debug API
// @version 1.0.0
// @description JFTrade sidecar API 的调试文档。Swagger UI 主要覆盖当前常用的 HTTP 调试入口，并展示实时接口的请求方式。
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey WebSession
// @in cookie
// @name jftrade_web_session
// @description 开启 Web 访问后，通过 Web 访问密码登录获得的浏览器会话。桌面应用不需要用户凭证。
