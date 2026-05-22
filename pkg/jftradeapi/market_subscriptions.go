package jftradeapi

import (
	"encoding/json"
	"net/http"
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

func (s *Server) handleAcquireMarketSubscription(w http.ResponseWriter, r *http.Request) {
	var payload marketSubscriptionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	response, err := s.acquireMarketSubscription(payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "market and symbol are required")
		return
	}

	s.writeOK(w, response)
}

func (s *Server) handleReleaseMarketSubscription(w http.ResponseWriter, r *http.Request) {
	var payload marketSubscriptionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	s.writeOK(w, s.releaseMarketSubscription(payload))
}

func (s *Server) handleHeartbeatMarketSubscription(w http.ResponseWriter, r *http.Request) {
	var payload marketSubscriptionHeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	s.writeOK(w, s.heartbeatMarketSubscriptions(payload.ConsumerID))
}

func (s *Server) handleClearMarketSubscriptions(w http.ResponseWriter, r *http.Request) {
	s.writeOK(w, s.clearMarketSubscriptions(r.URL.Query().Get("consumerId")))
}
