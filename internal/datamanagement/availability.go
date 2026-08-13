package datamanagement

// DatabaseID identifies a persisted application database at route and startup
// boundaries without exposing a concrete SQLite implementation.
type DatabaseID string

const (
	DatabaseBacktest         DatabaseID = "backtest"
	DatabaseBacktestRuns     DatabaseID = "backtest-runs"
	DatabaseStrategy         DatabaseID = "strategy"
	DatabaseExecution        DatabaseID = "execution-orders"
	DatabaseADK              DatabaseID = "adk"
	DatabaseADKSession       DatabaseID = "adk-session"
	DatabaseADKArtifact      DatabaseID = "adk-artifact"
	DatabaseWatchlist        DatabaseID = "watchlist"
	DatabaseResearch         DatabaseID = "research"
	DatabaseRealTradeControl DatabaseID = "real-trade-control"
)

// Availability is the read-only database readiness contract used by route
// registration. Implementations return nil when the database is available.
type Availability interface {
	Unavailable(DatabaseID) error
}

// AvailabilitySnapshot is the mutable startup snapshot populated by the
// composition root and consumed through the Availability interface.
type AvailabilitySnapshot map[DatabaseID]error

func NewAvailabilitySnapshot() AvailabilitySnapshot {
	return make(AvailabilitySnapshot)
}

func (snapshot AvailabilitySnapshot) Record(id DatabaseID, err error) {
	if snapshot != nil && err != nil {
		snapshot[id] = err
	}
}

func (snapshot AvailabilitySnapshot) Unavailable(id DatabaseID) error {
	return snapshot[id]
}
