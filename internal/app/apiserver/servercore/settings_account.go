package servercore

import (
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func normalizeManagedBrokerAccount(input jfsettings.ManagedBrokerAccount) jfsettings.ManagedBrokerAccount {
	return settingsfile.NormalizeManagedBrokerAccount(input)
}
