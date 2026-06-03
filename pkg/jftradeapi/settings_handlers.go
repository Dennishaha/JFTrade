package jftradeapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

type managedBrokerAccountWriteRequest struct {
	BrokerID           string `json:"brokerId"`
	AccountID          string `json:"accountId"`
	DisplayName        string `json:"displayName"`
	TradingEnvironment string `json:"tradingEnvironment"`
	Market             string `json:"market"`
	SecurityFirm       string `json:"securityFirm"`
	Enabled            bool   `json:"enabled"`
}

func (payload managedBrokerAccountWriteRequest) toManagedBrokerAccount() ManagedBrokerAccount {
	return ManagedBrokerAccount{
		BrokerID:           payload.BrokerID,
		AccountID:          payload.AccountID,
		DisplayName:        payload.DisplayName,
		TradingEnvironment: payload.TradingEnvironment,
		Market:             payload.Market,
		SecurityFirm:       stringPointerOrNil(payload.SecurityFirm),
		Enabled:            payload.Enabled,
	}
}

func (s *Server) handleSaveBrokerIntegration(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Enabled bool                  `json:"enabled"`
		Config  FutuIntegrationConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	integration, err := s.store.saveIntegration(BrokerIntegration{BrokerID: "futu", Enabled: payload.Enabled, Config: payload.Config})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, integration)
}

func (s *Server) handleSaveUIAppearance(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Appearance UIAppearanceSettings `json:"appearance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	appearance, err := s.store.saveAppearance(payload.Appearance)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}

	s.writeOK(w, map[string]any{"appearance": appearance})
}

func (s *Server) handleSaveOnboarding(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Completed    bool   `json:"completed"`
		Dismissed    bool   `json:"dismissed"`
		LastBrokerID string `json:"lastBrokerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	existing := s.store.onboarding()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next := existing
	next.LastBrokerID = payload.LastBrokerID
	if strings.TrimSpace(next.LastBrokerID) == "" {
		next.LastBrokerID = existing.LastBrokerID
	}
	if payload.Completed || payload.Dismissed {
		next.Completed = true
		if payload.Dismissed {
			next.DismissedAt = now
		}
		if next.CompletedAt == "" {
			next.CompletedAt = now
		}
	} else {
		next.Completed = false
		next.CompletedAt = ""
		next.DismissedAt = ""
	}

	onboarding, err := s.store.saveOnboarding(next)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, s.onboardingStateFromSettings(r.Context(), onboarding))
}

func (s *Server) handleCreateManagedBrokerAccount(w http.ResponseWriter, r *http.Request) {
	var payload managedBrokerAccountWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(payload.AccountID) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "accountId is required")
		return
	}
	account, err := s.store.createManagedAccount(payload.toManagedBrokerAccount())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, account)
}

func (s *Server) handleUpdateManagedBrokerAccount(w http.ResponseWriter, r *http.Request) {
	accountID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if accountID == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is required")
		return
	}
	var payload managedBrokerAccountWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	account, err := s.store.updateManagedAccount(accountID, payload.toManagedBrokerAccount())
	if errors.Is(err, os.ErrNotExist) {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "managed broker account not found")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, account)
}

func (s *Server) handleDeleteManagedBrokerAccount(w http.ResponseWriter, r *http.Request) {
	accountID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if accountID == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is required")
		return
	}
	if err := s.store.deleteManagedAccount(accountID); errors.Is(err, os.ErrNotExist) {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "managed broker account not found")
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, map[string]any{"deleted": true, "id": accountID})
}
