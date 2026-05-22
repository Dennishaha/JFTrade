package jftradeapi

import "strings"

func normalizeManagedBrokerAccount(input ManagedBrokerAccount) ManagedBrokerAccount {
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

func sameManagedAccountScope(left ManagedBrokerAccount, right ManagedBrokerAccount) bool {
	return left.BrokerID == right.BrokerID &&
		left.AccountID == right.AccountID &&
		left.TradingEnvironment == right.TradingEnvironment &&
		left.Market == right.Market
}

func buildManagedAccountID(input ManagedBrokerAccount) string {
	return strings.Join([]string{input.BrokerID, input.TradingEnvironment, input.AccountID, input.Market}, "|")
}
