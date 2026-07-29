package servercore

import (
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

func normalizeManagedBrokerAccount(input jfsettings.ManagedBrokerAccount) jfsettings.ManagedBrokerAccount {
	return settingsfile.NormalizeManagedBrokerAccount(input)
}
