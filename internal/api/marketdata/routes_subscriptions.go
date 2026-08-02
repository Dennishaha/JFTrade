package marketdata

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/marketdata"
)

type subscriptionRequest struct {
	ConsumerID       string              `json:"consumerId"`
	ProviderBrokerID string              `json:"providerBrokerId,omitempty"`
	Instruments      []srv.InstrumentRef `json:"instruments"`
}

// handleGetSubscriptions godoc
// @Summary 查询当前行情订阅
// @Tags market-data
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=SubscriptionsData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/subscriptions [get]
func handleGetSubscriptions(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := svc.GetSubscriptions(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleAcquireSubscription godoc
// @Summary 申请行情订阅
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body SubscriptionRequest true "订阅请求"
// @Success 200 {object} httpserver.Envelope{data=SubscriptionsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/subscriptions [post]
//
// 请求格式：
//
//	{"consumerId":"...", "instruments":[{"market":"...", "symbol":"...", "channel":"...", "interval":"..."}]}
func handleAcquireSubscription(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req subscriptionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid subscription request")
			return
		}
		consumerID := req.ConsumerID
		instruments := subscriptionInstruments(req)
		if consumerID == "" || len(instruments) == 0 {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "consumerId and instruments are required")
			return
		}
		if err := srv.ValidateSubscriptionRefs(instruments); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", err.Error())
			return
		}
		if usesBrokerPolling(req.ProviderBrokerID) {
			httpserver.WriteOK(c, brokerPollingSubscriptionResponse(
				req.ConsumerID, req.ProviderBrokerID, instruments, "acquired",
			))
			return
		}
		result, err := svc.AcquireSubscription(c.Request.Context(), consumerID, instruments)
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleReleaseSubscription godoc
// @Summary 释放行情订阅
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body SubscriptionRequest true "释放请求"
// @Success 200 {object} httpserver.Envelope{data=SubscriptionsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/subscriptions/release [post]
//
// 请求格式：
//
//	释放指定目标：{"consumerId":"...", "instruments":[{"market":"...", "symbol":"...", "channel":"...", "interval":"..."}]}
//	释放消费者全部订阅：{"consumerId":"..."}
func handleReleaseSubscription(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req subscriptionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid release request")
			return
		}
		consumerID := req.ConsumerID
		if consumerID == "" {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "consumerId is required")
			return
		}
		target, hasTarget, validTarget := subscriptionReleaseTarget(req)
		if !validTarget {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "release target market and symbol are required")
			return
		}
		if hasTarget {
			if err := srv.ValidateSubscriptionRefs([]srv.InstrumentRef{target}); err != nil {
				httpserver.WriteError(c, 400, "BAD_REQUEST", err.Error())
				return
			}
		}
		if usesBrokerPolling(req.ProviderBrokerID) {
			httpserver.WriteOK(c, brokerPollingSubscriptionResponse(
				req.ConsumerID, req.ProviderBrokerID, req.Instruments, "released",
			))
			return
		}
		var err error
		if hasTarget {
			err = svc.ReleaseSubscription(c.Request.Context(), consumerID, target)
		} else {
			err = svc.ReleaseSubscription(c.Request.Context(), consumerID)
		}
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result, err := svc.GetSubscriptions(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result["released"] = true
		httpserver.WriteOK(c, result)
	}
}

// handleClearSubscriptions godoc
// @Summary 清空行情订阅
// @Tags market-data
// @Produce json
// @Param consumerId query string false "消费者 ID；为空时清空全部"
// @Success 200 {object} httpserver.Envelope{data=SubscriptionsData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/subscriptions [delete]
func handleClearSubscriptions(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.ClearSubscriptions(c.Request.Context(), c.Query("consumerId")); err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result, err := svc.GetSubscriptions(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result["cleared"] = true
		httpserver.WriteOK(c, result)
	}
}

func subscriptionInstruments(req subscriptionRequest) []srv.InstrumentRef {
	instruments := make([]srv.InstrumentRef, 0, len(req.Instruments))
	for _, instrument := range req.Instruments {
		if strings.TrimSpace(instrument.Market) == "" || strings.TrimSpace(instrument.Symbol) == "" {
			continue
		}
		instruments = append(instruments, instrument)
	}
	if len(instruments) > 0 {
		return instruments
	}
	return nil
}

func subscriptionReleaseTarget(req subscriptionRequest) (srv.InstrumentRef, bool, bool) {
	if len(req.Instruments) == 0 {
		return srv.InstrumentRef{}, false, true
	}
	target := req.Instruments[0]
	if strings.TrimSpace(target.Market) == "" || strings.TrimSpace(target.Symbol) == "" {
		return srv.InstrumentRef{}, false, false
	}
	return target, true, true
}

// handleHeartbeat godoc
// @Summary 刷新订阅心跳
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body SubscriptionHeartbeatRequest true "心跳请求"
// @Success 200 {object} httpserver.Envelope{data=SubscriptionsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/subscriptions/heartbeat [post]
func handleHeartbeat(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ConsumerID       string `json:"consumerId"`
			ProviderBrokerID string `json:"providerBrokerId,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid heartbeat request")
			return
		}
		if strings.TrimSpace(req.ConsumerID) == "" {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "consumerId is required")
			return
		}
		if usesBrokerPolling(req.ProviderBrokerID) {
			httpserver.WriteOK(c, brokerPollingSubscriptionResponse(
				req.ConsumerID, req.ProviderBrokerID, nil, "heartbeat",
			))
			return
		}
		result, err := svc.Heartbeat(c.Request.Context(), req.ConsumerID)
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func usesBrokerPolling(brokerID string) bool {
	brokerID = strings.TrimSpace(brokerID)
	return brokerID != "" && !strings.EqualFold(brokerID, "futu")
}

func brokerPollingSubscriptionResponse(
	consumerID string,
	brokerID string,
	instruments []srv.InstrumentRef,
	action string,
) map[string]any {
	return map[string]any{
		"consumerId":               consumerID,
		"providerBrokerId":         strings.ToLower(strings.TrimSpace(brokerID)),
		"instruments":              instruments,
		"action":                   action,
		"totalActiveSubscriptions": 0,
		"desiredCount":             0,
		"ownActiveCount":           0,
		"pendingReleaseCount":      0,
		"entries":                  []any{},
		"quota": map[string]any{
			"totalUsed":      0,
			"totalLimit":     nil,
			"totalRemaining": nil,
			"byMarket":       []any{},
		},
		"transport": map[string]any{
			"mode": "snapshot-poll-fallback",
		},
	}
}
