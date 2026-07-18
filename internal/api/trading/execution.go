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
	executionProductRoutesDocs()
	executionComboPreviewDocs()
	executionComboPlaceDocs()
	executionComboCancelDocs()
	executionBuyingPowerDocs()
	api.GET("/execution/orders", handleExecutionOrders(service))
	api.POST("/execution/orders", handleExecutionPlace(service))
	api.POST("/execution/orders/preview", handleExecutionPreview(service))
	api.POST("/execution/previews", handleExecutionPreview(service))
	api.POST("/execution/combos/previews", handleExecutionComboPreview(service))
	api.POST("/execution/combos", handleExecutionComboPlace(service))
	api.POST("/execution/combos/:internalOrderId/cancel", handleExecutionComboCancel(service))
	api.POST("/execution/buying-power", handleExecutionBuyingPower(service))
	api.GET("/execution/orders/:internalOrderId", handleExecutionOrderDetails(service))
	api.GET("/execution/orders/:internalOrderId/events", handleExecutionEvents(service))
	api.POST("/execution/orders/:internalOrderId/cancel", handleExecutionCancel(service))
}

func handleExecutionBuyingPower(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request broker.ProductRuleQuery
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid buying-power request")
			return
		}
		result, err := service.PreviewExecutionBuyingPower(c.Request.Context(), request)
		if err != nil {
			status, code := executionCommandError(err)
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionComboPreview(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request srv.ExecutionComboRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid combo preview payload")
			return
		}
		result, err := service.PreviewExecutionCombo(c.Request.Context(), request)
		if err != nil {
			status, code := executionCommandError(err)
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionComboPlace(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request srv.ExecutionComboRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid combo order payload")
			return
		}
		result, err := service.CreateExecutionCombo(c.Request.Context(), request)
		if err != nil {
			status, code := executionCommandError(err)
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionComboCancel(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := bindInternalOrderID(c)
		if !ok {
			return
		}
		result, err := service.CancelExecutionCombo(c.Request.Context(), id)
		if err != nil {
			status, code := executionCommandError(err)
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionOrderDetails(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := bindInternalOrderID(c)
		if !ok {
			return
		}
		result, err := service.ExecutionOrderDetails(c.Request.Context(), id)
		if errors.Is(err, srv.ErrExecutionOrderNotFound) {
			httpserver.WriteError(c, http.StatusNotFound, "ORDER_NOT_FOUND", err.Error())
			return
		}
		if err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, "GET_ORDER_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleExecutionOrders(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := executionOrdersQuery{
			Scope:              c.Query("scope"),
			BrokerID:           c.Query("brokerId"),
			TradingEnvironment: c.Query("tradingEnvironment"),
			AccountID:          c.Query("accountId"),
			Market:             c.Query("market"),
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
		result, err := service.PreviewExecutionOrderContext(c.Request.Context(), request)
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
	var riskErr srv.RiskRejectedError
	if errors.As(err, &riskErr) && riskErr.Decision.RequiresApproval() {
		return http.StatusConflict, "PRE_TRADE_APPROVAL_REQUIRED"
	}
	if srv.IsRiskRejected(err) {
		return http.StatusConflict, "PRE_TRADE_RISK_REJECTED"
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

// executionProductRoutesDocs godoc
// @Summary 全产品组合交易、预检和购买力
// @Tags execution
// @Accept json
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/execution/previews [post]
func executionProductRoutesDocs() {}

// executionComboPreviewDocs godoc
// @Summary 预检组合订单
// @Description 锁定券商、账户、组合腿和能力版本，返回组合风险与账户购买力影响，不提交订单。
// @Tags execution
// @Accept json
// @Produce json
// @Param request body srv.ExecutionComboRequest true "组合预检请求"
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionComboPreview}
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/execution/combos/previews [post]
func executionComboPreviewDocs() {}

// executionComboPlaceDocs godoc
// @Summary 提交已预检的组合订单
// @Description 仅接受未过期且与当前请求完全匹配的 preview，使用稳定 clientOrderId 保证幂等。
// @Tags execution
// @Accept json
// @Produce json
// @Param request body srv.ExecutionComboRequest true "组合下单请求"
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionCommandResponse}
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/execution/combos [post]
func executionComboPlaceDocs() {}

// executionComboCancelDocs godoc
// @Summary 撤销组合订单
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "父组合订单内部编号"
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionCommandResponse}
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/execution/combos/{internalOrderId}/cancel [post]
func executionComboCancelDocs() {}

// executionBuyingPowerDocs godoc
// @Summary 查询产品订单购买力
// @Tags execution
// @Accept json
// @Produce json
// @Param request body broker.ProductRuleQuery true "购买力查询"
// @Success 200 {object} httpserver.Envelope{data=broker.ProductRuleResult}
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/execution/buying-power [post]
func executionBuyingPowerDocs() {}
