package settingsfile

import (
	"errors"
	"os"
	"strings"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const (
	defaultFutuHost            = "127.0.0.1"
	defaultFutuAPIPort         = 11110
	defaultFutuWebSocketPort   = 11111
	defaultMaxWebSocketClients = 20
)

func (s *Store) Integration() jfsettings.BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration != nil {
		return *s.data.Integration
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return jfsettings.BrokerIntegration{
		BrokerID:  "futu",
		Enabled:   false,
		Config:    DefaultFutuConfig(),
		UpdatedAt: now,
		CreatedAt: now,
	}
}

func (s *Store) SavedIntegration() *jfsettings.BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration == nil {
		return nil
	}
	return new(*s.data.Integration)
}

func (s *Store) SaveIntegration(input jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.BrokerID = "futu"
	input.Config = NormalizeFutuConfig(input.Config)
	input.UpdatedAt = now
	if input.CreatedAt == "" {
		if existing := s.SavedIntegration(); existing != nil {
			input.CreatedAt = existing.CreatedAt
		}
		if input.CreatedAt == "" {
			input.CreatedAt = now
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndPersistLocked(func() {
		if s.data.Interfaces == nil {
			s.data.Interfaces = interfaceSettingsPointer(NormalizeInterfaceSettings(InterfaceSettingsFromDefaults(jfsettings.LaunchDefaults{}), jfsettings.LaunchDefaults{}))
		}
		s.data.Integration = &input
	})
	if err != nil {
		return input, err
	}

	return input, nil
}

func (s *Store) ManagedAccounts() []jfsettings.ManagedBrokerAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := make([]jfsettings.ManagedBrokerAccount, len(s.data.Accounts))
	copy(accounts, s.data.Accounts)
	return accounts
}

func (s *Store) CreateManagedAccount(input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	input = NormalizeManagedBrokerAccount(input)
	if input.AccountID == "" {
		return jfsettings.ManagedBrokerAccount{}, errors.New("accountId is required")
	}
	input.ID = ""
	input.CreatedAt = ""
	input.UpdatedAt = ""
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if SameManagedAccountScope(*account, input) {
			input.ID = account.ID
			input.CreatedAt = account.CreatedAt
			if input.CreatedAt == "" {
				input.CreatedAt = now
			}
			if err := s.mutateAndPersistLocked(func() {
				s.data.Accounts[index] = input
			}); err != nil {
				return input, err
			}
			return input, nil
		}
	}

	if input.ID == "" {
		input.ID = BuildManagedAccountID(input)
	}
	if input.CreatedAt == "" {
		input.CreatedAt = now
	}
	if err := s.mutateAndPersistLocked(func() {
		s.data.Accounts = append(s.data.Accounts, input)
	}); err != nil {
		return input, err
	}
	return input, nil
}

func (s *Store) UpdateManagedAccount(id string, input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	input = NormalizeManagedBrokerAccount(input)
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
		if err := s.mutateAndPersistLocked(func() {
			s.data.Accounts[index] = input
		}); err != nil {
			return input, err
		}
		return input, nil
	}

	return jfsettings.ManagedBrokerAccount{}, os.ErrNotExist
}

func (s *Store) DeleteManagedAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		if s.data.Accounts[index].ID != id {
			continue
		}
		return s.mutateAndPersistLocked(func() {
			s.data.Accounts = append(s.data.Accounts[:index], s.data.Accounts[index+1:]...)
		})
	}
	return os.ErrNotExist
}

func NormalizeManagedBrokerAccount(input jfsettings.ManagedBrokerAccount) jfsettings.ManagedBrokerAccount {
	input.BrokerID = strings.TrimSpace(strings.ToLower(input.BrokerID))
	if input.BrokerID == "" {
		input.BrokerID = "futu"
	}
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		input.DisplayName = input.AccountID
	}
	input.TradingEnvironment = strings.ToUpper(strings.TrimSpace(input.TradingEnvironment))
	if input.TradingEnvironment == "" {
		input.TradingEnvironment = "SIMULATE"
	}
	input.Market = strings.ToUpper(strings.TrimSpace(input.Market))
	if input.Market == "" {
		input.Market = "HK"
	}
	if input.SecurityFirm != nil {
		value := strings.TrimSpace(*input.SecurityFirm)
		if value == "" {
			input.SecurityFirm = nil
		} else {
			input.SecurityFirm = &value
		}
	}
	return input
}

func SameManagedAccountScope(left jfsettings.ManagedBrokerAccount, right jfsettings.ManagedBrokerAccount) bool {
	return left.BrokerID == right.BrokerID &&
		left.AccountID == right.AccountID &&
		left.TradingEnvironment == right.TradingEnvironment &&
		left.Market == right.Market
}

func BuildManagedAccountID(input jfsettings.ManagedBrokerAccount) string {
	return strings.Join([]string{input.BrokerID, input.TradingEnvironment, input.AccountID, input.Market}, "|")
}

func DefaultFutuConfig() jfsettings.FutuIntegrationConfig {
	return NormalizeFutuConfig(jfsettings.FutuIntegrationConfig{
		Type:                    "futu",
		Host:                    defaultFutuHost,
		APIPort:                 defaultFutuAPIPort,
		WebSocketPort:           defaultFutuWebSocketPort,
		MaxWebSocketConnections: defaultMaxWebSocketClients,
		UseEncryption:           false,
		TradeMarket:             "HK",
		SecurityFirm:            "FUTUSECURITIES",
	})
}

func NormalizeFutuConfig(config jfsettings.FutuIntegrationConfig) jfsettings.FutuIntegrationConfig {
	if config.Type == "" {
		config.Type = "futu"
	}
	if strings.TrimSpace(config.Host) == "" {
		config.Host = defaultFutuHost
	}
	if config.APIPort <= 0 {
		config.APIPort = defaultFutuAPIPort
	}
	if config.WebSocketPort <= 0 {
		config.WebSocketPort = defaultFutuWebSocketPort
	}
	if config.MaxWebSocketConnections <= 0 {
		config.MaxWebSocketConnections = defaultMaxWebSocketClients
	}
	if strings.TrimSpace(config.TradeMarket) == "" {
		config.TradeMarket = "HK"
	}
	if strings.TrimSpace(config.SecurityFirm) == "" {
		config.SecurityFirm = "FUTUSECURITIES"
	}
	config.UseEncryption = false
	return config
}
