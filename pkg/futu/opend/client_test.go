package opend

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
)

// startFakeOpenD serves a single connection that echoes back a response
// containing the requested protoID/serialNo and a small InitConnect.Response.
func startFakeOpenD(t *testing.T) (addr string, stop func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			f, err := codec.Decode(data)
			if err != nil {
				return
			}
			resp := &initpb.Response{
				RetType: proto.Int32(0),
				S2C: &initpb.S2C{
					ServerVer:         proto.Int32(700),
					LoginUserID:       proto.Uint64(1),
					ConnID:            proto.Uint64(42),
					ConnAESKey:        proto.String("0123456789abcdef"),
					KeepAliveInterval: proto.Int32(10),
				},
			}
			body, _ := proto.Marshal(resp)
			pkt, _ := codec.Encode(f.Header.ProtoID, f.Header.SerialNo, body)
			_ = c.WriteMessage(websocket.BinaryMessage, pkt)
		}
	}))
	// httptest gives http://host:port; we need host:port for our ws config.
	u := strings.TrimPrefix(srv.URL, "http://")
	return u, srv.Close
}

func TestCallRoundTrip(t *testing.T) {
	addr, stop := startFakeOpenD(t)
	defer stop()

	c := New(Config{Addr: addr, RequestTimeout: 3 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	req := &initpb.Request{C2S: &initpb.C2S{
		ClientVer: proto.Int32(101),
		ClientID:  proto.String("jftrade-test"),
	}}
	var resp initpb.Response
	if err := c.Call(ctx, ProtoInitConnect, req, &resp); err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.GetS2C().GetConnID() != 42 {
		t.Fatalf("unexpected resp: %+v", &resp)
	}
}

func TestRequestTimeout(t *testing.T) {
	// listen-only socket: never responds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// hold connection without responding
		_, _, _ = c.ReadMessage()
		time.Sleep(2 * time.Second)
		_ = c.Close()
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	c := New(Config{Addr: ln.Addr().String(), RequestTimeout: 200 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()
	var resp initpb.Response
	err = c.Call(ctx, ProtoInitConnect, &initpb.Request{C2S: &initpb.C2S{
		ClientVer: proto.Int32(1), ClientID: proto.String("x"),
	}}, &resp)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
