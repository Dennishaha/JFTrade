package servercore

import "github.com/jftrade/jftrade-main/internal/store/settingsfile"

func normalizeManagedBrokerAccount(input ManagedBrokerAccount) ManagedBrokerAccount {
	return settingsfile.NormalizeManagedBrokerAccount(input)
}

func sameManagedAccountScope(left ManagedBrokerAccount, right ManagedBrokerAccount) bool {
	return settingsfile.SameManagedAccountScope(left, right)
}

func buildManagedAccountID(input ManagedBrokerAccount) string {
	return settingsfile.BuildManagedAccountID(input)
}
