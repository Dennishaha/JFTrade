package opend

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
)

// startFakeOpenD serves a single TCP connection that echoes back a response
// containing the requested protoID/serialNo and a small InitConnect.Response.
func startFakeOpenD(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			header := make([]byte, codec.HeaderLen)
			if _, err := io.ReadFull(conn, header); err != nil {
				return
			}
			bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
			packet := make([]byte, codec.HeaderLen+bodyLen)
			copy(packet, header)
			if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
				return
			}
			f, err := codec.Decode(packet)
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
			_, _ = conn.Write(pkt)
		}
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

func startKeepAliveStallOpenD(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for requestIndex := 0; ; requestIndex++ {
			header := make([]byte, codec.HeaderLen)
			if _, err := io.ReadFull(conn, header); err != nil {
				return
			}
			bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
			packet := make([]byte, codec.HeaderLen+bodyLen)
			copy(packet, header)
			if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
				return
			}
			f, err := codec.Decode(packet)
			if err != nil {
				return
			}
			if requestIndex > 0 {
				continue
			}
			resp := &initpb.Response{
				RetType: proto.Int32(0),
				S2C: &initpb.S2C{
					ServerVer:         proto.Int32(700),
					LoginUserID:       proto.Uint64(1),
					ConnID:            proto.Uint64(42),
					ConnAESKey:        proto.String("0123456789abcdef"),
					KeepAliveInterval: proto.Int32(1),
				},
			}
			body, _ := proto.Marshal(resp)
			pkt, _ := codec.Encode(f.Header.ProtoID, f.Header.SerialNo, body)
			_, _ = conn.Write(pkt)
		}
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, codec.HeaderLen+256)
		_, _ = conn.Read(buf)
		time.Sleep(2 * time.Second)
	}()

	c := New(Config{Addr: ln.Addr().String(), RequestTimeout: 200 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()
	var resp initpb.Response
	err = c.Call(ctx, ProtoInitConnect, &initpb.Request{C2S: &initpb.C2S{
		ClientVer: proto.Int32(1),
		ClientID:  proto.String("x"),
	}}, &resp)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestKeepAliveFailureClosesClient(t *testing.T) {
	addr, stop := startKeepAliveStallOpenD(t)
	defer stop()

	c := New(Config{Addr: addr, RequestTimeout: 30 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	var initResp initpb.Response
	if err := c.Call(ctx, ProtoInitConnect, &initpb.Request{C2S: &initpb.C2S{
		ClientVer: proto.Int32(1),
		ClientID:  proto.String("x"),
	}}, &initResp); err != nil {
		t.Fatalf("init call: %v", err)
	}
	c.StartKeepAlive(20 * time.Millisecond)

	select {
	case <-c.Done():
	case <-ctx.Done():
		t.Fatal("timed out waiting for keepalive to close client")
	}

	var resp initpb.Response
	err := c.Call(ctx, ProtoInitConnect, &initpb.Request{C2S: &initpb.C2S{
		ClientVer: proto.Int32(1),
		ClientID:  proto.String("x"),
	}}, &resp)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed after keepalive failure, got %v", err)
	}
}
