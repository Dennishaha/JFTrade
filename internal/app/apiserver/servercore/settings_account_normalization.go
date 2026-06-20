package servercore

import "github.com/jftrade/jftrade-main/internal/store/settingsfile"

func normalizeManagedBrokerAccount(input ManagedBrokerAccount) ManagedBrokerAccount {
	return settingsfile.NormalizeManagedBrokerAccount(input)
}
