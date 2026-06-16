package trading

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type executionOrdersQuery struct {
	Scope              string `form:"scope"`
	BrokerID           string `form:"brokerId"`
	TradingEnvironment string `form:"tradingEnvironment"`
	AccountID          string `form:"accountId"`
	Market             string `form:"market"`
}

type internalOrderURI struct {
	InternalOrderID string `uri:"internalOrderId" binding:"required"`
}

func RegisterExecutionRoutes(api *gin.RouterGroup, service *srv.Service) {
	api.GET("/execution/orders", handleExecutionOrders(service))
	api.POST("/execution/orders", handleExecutionPlace(service))
	api.POST("/execution/orders/preview", handleExecutionPreview(service))
	api.GET("/execution/orders/:internalOrderId/events", handleExecutionEvents(service))
	api.POST("/execution/orders/:internalOrderId/cancel", handleExecutionCancel(service))
}

func handleExecutionOrders(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query executionOrdersQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid execution query")
			return
		}
		activeOnly := strings.EqualFold(strings.TrimSpace(query.Scope), "ACTIVE")
		filter := service.ExecutionFilter(
			query.BrokerID, query.TradingEnvironment, query.AccountID, query.Market,
		)
		result, err := service.ListExecutionOrders(c.Request.Context(), filter, activeOnly)
		if err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, "LIST_ORDERS_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionEvents(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := bindInternalOrderID(c)
		if !ok {
			return
		}
		result, err := service.ExecutionOrderEvents(c.Request.Context(), id)
		if err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, "GET_ORDER_EVENTS_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionPlace(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request srv.ExecutionPlaceRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid execution order payload")
			return
		}
		result, err := service.CreateExecutionOrder(c.Request.Context(), request)
		if err != nil {
			status, code := executionCommandError(err)
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionPreview(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request srv.ExecutionPlaceRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid execution order payload")
			return
		}
		result, err := service.PreviewExecutionOrder(request)
		if err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionCancel(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := bindInternalOrderID(c)
		if !ok {
			return
		}
		result, err := service.CancelExecutionOrder(c.Request.Context(), id)
		if err != nil {
			status, code := executionCommandError(err)
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func bindInternalOrderID(c *gin.Context) (string, bool) {
	var uri internalOrderURI
	if err := httpserver.BindURI(c, &uri); err != nil {
		httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "internalOrderId is invalid")
		return "", false
	}
	return strings.TrimSpace(uri.InternalOrderID), true
}

func executionCommandError(err error) (int, string) {
	if srv.IsRequestError(err) {
		return http.StatusBadRequest, "BAD_REQUEST"
	}
	if brokerErr, ok := errors.AsType[*broker.BrokerError](err); ok {
		switch strings.TrimSpace(brokerErr.Code) {
		case broker.ErrCodeAccountNotFound, broker.ErrCodeMarketNotSupported, broker.ErrCodeOrderNotFound:
			return http.StatusBadRequest, "BAD_REQUEST"
		case broker.ErrCodeTimeout:
			return http.StatusGatewayTimeout, "BROKER_TIMEOUT"
		case broker.ErrCodeRateLimited:
			return http.StatusTooManyRequests, "BROKER_RATE_LIMITED"
		case broker.ErrCodeNotConnected:
			return http.StatusBadGateway, "BROKER_NOT_CONNECTED"
		default:
			return http.StatusBadGateway, "BROKER_COMMAND_FAILED"
		}
	}
	return http.StatusBadGateway, "BROKER_COMMAND_FAILED"
}
