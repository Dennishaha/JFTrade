package futu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestClassifyBasicQotSubscriptionErrorOnlyMarksAvailabilityFailures(t *testing.T) {
	availability := broker.NewSnapshotAvailabilityError(
		broker.SnapshotAvailabilityQuota,
		errors.New("already classified"),
	)
	for _, test := range []struct {
		name string
		err  error
		kind broker.SnapshotAvailabilityKind
	}{
		{name: "nil"},
		{name: "cancelled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "rate limited", err: broker.NewSnapshotRateLimitError(time.Second, nil)},
		{name: "already classified", err: availability, kind: broker.SnapshotAvailabilityQuota},
		{name: "frequency", err: errors.New("request frequency too high")},
		{name: "entitlement", err: errors.New("quote right permission denied"), kind: broker.SnapshotAvailabilityEntitlement},
		{name: "quota", err: errors.New("subscription is full"), kind: broker.SnapshotAvailabilityQuota},
		{name: "unsupported", err: errors.New("unknown stock"), kind: broker.SnapshotAvailabilityUnsupported},
		{name: "other", err: errors.New("OpenD transport failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := classifyBasicQotSubscriptionError(test.err)
			if test.err == nil {
				if got != nil {
					t.Fatalf("nil error classified as %v", got)
				}
				return
			}
			if !errors.Is(got, test.err) {
				t.Fatalf("classified error does not retain cause: %v", got)
			}
			kind, ok := broker.SnapshotAvailability(got)
			if test.kind == "" {
				if ok || broker.IsSnapshotFallbackEligible(got) {
					t.Fatalf("non-availability error was classified: %q, %t", kind, ok)
				}
				return
			}
			if !ok || kind != test.kind || !broker.IsSnapshotFallbackEligible(got) {
				t.Fatalf("availability classification = %q, %t for %v", kind, ok, got)
			}
		})
	}
}
