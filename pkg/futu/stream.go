package futu

import (
	"context"

	"github.com/c9s/bbgo/pkg/types"
)

// Stream is a placeholder bbgo stream backed by Futu OpenD. The standard
// stream provides the bulk of the interface; full push-event wiring will be
// implemented incrementally without modifying bbgo internals.
type Stream struct {
	types.StandardStream

	exchange *Exchange
}

// NewStream constructs a Stream tied to the given Exchange.
func NewStream(ex *Exchange) *Stream {
	s := &Stream{StandardStream: types.NewStandardStream(), exchange: ex}
	s.SetEndpointCreator(func(ctx context.Context) (string, error) {
		// Re-use the OpenD WebSocket address. Real push wiring will be added
		// when proto handlers are mapped.
		return "ws://" + ex.addr, nil
	})
	return s
}
