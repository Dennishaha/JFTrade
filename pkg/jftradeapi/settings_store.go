package jftradeapi

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FutuIntegrationConfig struct {
	Type                    string `json:"type"`
	Host                    string `json:"host"`
	APIPort                 int    `json:"apiPort"`
	WebSocketPort           int    `json:"websocketPort"`
	MaxWebSocketConnections int    `json:"maxWebSocketConnections"`
	UseEncryption           bool   `json:"useEncryption"`
	WebSocketKey            string `json:"websocketKey"`
	TradeMarket             string `json:"tradeMarket"`
	SecurityFirm            string `json:"securityFirm"`
}

type BrokerIntegration struct {
	BrokerID  string                `json:"brokerId"`
	Enabled   bool                  `json:"enabled"`
	Config    FutuIntegrationConfig `json:"config"`
	UpdatedAt string                `json:"updatedAt"`
	CreatedAt string                `json:"createdAt"`
}

type ManagedBrokerAccount struct {
	ID                 string  `json:"id"`
	BrokerID           string  `json:"brokerId"`
	AccountID          string  `json:"accountId"`
	DisplayName        string  `json:"displayName"`
	TradingEnvironment string  `json:"tradingEnvironment"`
	Market             string  `json:"market"`
	SecurityFirm       *string `json:"securityFirm"`
	Enabled            bool    `json:"enabled"`
	UpdatedAt          string  `json:"updatedAt"`
	CreatedAt          string  `json:"createdAt"`
}

type settingsFile struct {
	Integration *BrokerIntegration     `json:"integration,omitempty"`
	Accounts    []ManagedBrokerAccount `json:"accounts,omitempty"`
}

type SettingsStore struct {
	path string
	mu   sync.RWMutex
	data settingsFile
}

func NewSettingsStore(path string) (*SettingsStore, error) {
	store := &SettingsStore{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SettingsStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = settingsFile{}
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.data = settingsFile{}
		return nil
	}
	return json.Unmarshal(data, &s.data)
}

func (s *SettingsStore) integration() BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration != nil {
		return *s.data.Integration
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return BrokerIntegration{
		BrokerID:  "futu",
		Enabled:   true,
		Config:    defaultFutuConfig(),
		UpdatedAt: now,
		CreatedAt: now,
	}
}

func (s *SettingsStore) saveIntegration(input BrokerIntegration) (BrokerIntegration, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.BrokerID = "futu"
	input.Config = normalizeFutuConfig(input.Config)
	input.UpdatedAt = now
	if input.CreatedAt == "" {
		existing := s.integration()
		input.CreatedAt = existing.CreatedAt
		if input.CreatedAt == "" {
			input.CreatedAt = now
		}
	}

	s.mu.Lock()
	s.data.Integration = &input
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		s.mu.Unlock()
		return input, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		s.mu.Unlock()
		return input, err
	}
	err = os.WriteFile(s.path, data, 0o600)
	s.mu.Unlock()
	if err != nil {
		return input, err
	}

	s.applyRuntimeEnv()
	return input, nil
}

func (s *SettingsStore) managedAccounts() []ManagedBrokerAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := make([]ManagedBrokerAccount, len(s.data.Accounts))
	copy(accounts, s.data.Accounts)
	return accounts
}

func (s *SettingsStore) createManagedAccount(input ManagedBrokerAccount) (ManagedBrokerAccount, error) {
	input = normalizeManagedBrokerAccount(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if sameManagedAccountScope(*account, input) {
			input.ID = account.ID
			input.CreatedAt = account.CreatedAt
			if input.CreatedAt == "" {
				input.CreatedAt = now
			}
			s.data.Accounts[index] = input
			if err := s.persistLocked(); err != nil {
				return input, err
			}
			return input, nil
		}
	}

	if input.ID == "" {
		input.ID = buildManagedAccountID(input)
	}
	if input.CreatedAt == "" {
		input.CreatedAt = now
	}
	s.data.Accounts = append(s.data.Accounts, input)
	if err := s.persistLocked(); err != nil {
		return input, err
	}
	return input, nil
}

func (s *SettingsStore) updateManagedAccount(id string, input ManagedBrokerAccount) (ManagedBrokerAccount, error) {
	input = normalizeManagedBrokerAccount(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if account.ID != id {
			continue
		}
		input.ID = account.ID
		input.CreatedAt = account.CreatedAt
		input.UpdatedAt = now
		s.data.Accounts[index] = input
		if err := s.persistLocked(); err != nil {
			return input, err
		}
		return input, nil
	}

	return ManagedBrokerAccount{}, os.ErrNotExist
}

func (s *SettingsStore) deleteManagedAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		if s.data.Accounts[index].ID != id {
			continue
		}
		s.data.Accounts = append(s.data.Accounts[:index], s.data.Accounts[index+1:]...)
		return s.persistLocked()
	}
	return os.ErrNotExist
}

func (s *SettingsStore) persistLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
