package futu

import (
	"context"
	"net"
	"net/url"
	"os"

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
		host, _, err := net.SplitHostPort(ex.addr)
		if err != nil {
			host = "127.0.0.1"
		}
		wsPort := os.Getenv("JFTRADE_FUTU_WEBSOCKET_PORT")
		if wsPort == "" {
			wsPort = "11111"
		}
		u := url.URL{Scheme: "ws", Host: net.JoinHostPort(host, wsPort), Path: "/"}
		if ex.webSocketKey != "" {
			q := u.Query()
			q.Set("key", ex.webSocketKey)
			u.RawQuery = q.Encode()
		}
		return u.String(), nil
	})
	return s
}
