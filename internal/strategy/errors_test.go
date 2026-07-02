package strategy

import (
	"errors"
	"testing"
)

func TestClassifiedStrategyErrorsMatchSentinelKinds(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "bad request", err: BadRequestError("invalid strategy"), want: ErrBadRequest},
		{name: "busy", err: BusyError("optimizer busy"), want: ErrBusy},
		{name: "not found", err: NotFoundError("strategy missing"), want: ErrNotFound},
		{name: "upstream", err: UpstreamError("pine worker failed"), want: ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.want) {
				t.Fatalf("errors.Is(%v, %v) = false", tc.err, tc.want)
			}
			if tc.err.Error() == "" {
				t.Fatal("classified error message should be preserved")
			}
		})
	}
}
