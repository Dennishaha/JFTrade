package strategy

import (
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

// Resource is the application-owned strategy design resource. Business code
// receives DesignStore; bootstrap additionally needs availability, maintenance
// and lifecycle capabilities without accessing Store internals.
type Resource interface {
	stratsrv.DesignStore
	dmsrv.CandidatePurger
	dmsrv.Compactor
	Available() bool
	Close() error
}

var (
	_ stratsrv.DesignStore  = (*Store)(nil)
	_ dmsrv.CandidatePurger = (*Store)(nil)
	_ dmsrv.Compactor       = (*Store)(nil)
	_ Resource              = (*Store)(nil)
)
