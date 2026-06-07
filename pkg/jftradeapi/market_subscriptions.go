package jftradeapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type marketSubscriptionPayload struct {
	Channel    string `json:"channel"`
	Market     string `json:"market"`
	Symbol     string `json:"symbol"`
	Interval   string `json:"interval"`
	ConsumerID string `json:"consumerId"`
}

type marketSubscriptionHeartbeatPayload struct {
	ConsumerID string `json:"consumerId"`
}

// handleAcquireMarketSubscription godoc
// @Summary 申请行情订阅
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body marketSubscriptionPayload true "订阅请求"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/market-data/subscriptions [post]
func (s *Server) handleAcquireMarketSubscription(c *gin.Context) {
	var payload marketSubscriptionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	response, err := s.acquireMarketSubscription(payload)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "market and symbol are required")
		return
	}

	s.writeOK(c, response)
}

// handleReleaseMarketSubscription godoc
// @Summary 释放行情订阅
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body marketSubscriptionPayload true "订阅请求"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/market-data/subscriptions/release [post]
func (s *Server) handleReleaseMarketSubscription(c *gin.Context) {
	var payload marketSubscriptionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	s.writeOK(c, s.releaseMarketSubscription(payload))
}

// handleHeartbeatMarketSubscription godoc
// @Summary 刷新订阅心跳
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body marketSubscriptionHeartbeatPayload true "订阅心跳"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/market-data/subscriptions/heartbeat [post]
func (s *Server) handleHeartbeatMarketSubscription(c *gin.Context) {
	var payload marketSubscriptionHeartbeatPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	s.writeOK(c, s.heartbeatMarketSubscriptions(payload.ConsumerID))
}

// handleClearMarketSubscriptions godoc
// @Summary 清空行情订阅
// @Tags market-data
// @Produce json
// @Param consumerId query string false "消费者 ID；为空时清空全部"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/market-data/subscriptions [delete]
func (s *Server) handleClearMarketSubscriptions(c *gin.Context) {
	var query marketSubscriptionDeleteQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid clear subscriptions query")
		return
	}
	s.writeOK(c, s.clearMarketSubscriptions(query.ConsumerID))
}
