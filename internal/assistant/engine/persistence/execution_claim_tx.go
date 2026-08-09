package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *StoreCore) lockRunLeaseFromContext(ctx context.Context, tx executionClaimTx, runID string) error {
	lease, ok := s.runLeaseFromContext(ctx)
	if !ok {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if lease.RunID != runID {
		return fmt.Errorf("%w: run %s cannot be written with lease for run %s", ErrRunLeaseLost, runID, lease.RunID)
	}
	return LockRunLease(ctx, tx, lease.RunID, lease.OwnerID, lease.FencingToken, time.Now().UTC())
}
