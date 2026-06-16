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
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
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
				RetType: new(int32(0)),
				S2C: &initpb.S2C{
					ServerVer:         new(int32(700)),
					LoginUserID:       new(uint64(1)),
					ConnID:            new(uint64(42)),
					ConnAESKey:        new("0123456789abcdef"),
					KeepAliveInterval: new(int32(10)),
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
				RetType: new(int32(0)),
				S2C: &initpb.S2C{
					ServerVer:         new(int32(700)),
					LoginUserID:       new(uint64(1)),
					ConnID:            new(uint64(42)),
					ConnAESKey:        new("0123456789abcdef"),
					KeepAliveInterval: new(int32(1)),
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
		ClientVer: new(int32(101)),
		ClientID:  new("jftrade-test"),
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
		ClientVer: new(int32(1)),
		ClientID:  new("x"),
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
		ClientVer: new(int32(1)),
		ClientID:  new("x"),
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
		ClientVer: new(int32(1)),
		ClientID:  new("x"),
	}}, &resp)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed after keepalive failure, got %v", err)
	}
}

func TestSubscribeNotifyReceivesSystemPush(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	recvNotifyCh := make(chan bool, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

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
		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}
		request := &initpb.Request{}
		if err := proto.Unmarshal(frame.Body, request); err != nil {
			return
		}
		recvNotifyCh <- request.GetC2S().GetRecvNotify()

		response := &initpb.Response{
			RetType: new(int32(0)),
			S2C: &initpb.S2C{
				ServerVer:         new(int32(700)),
				LoginUserID:       new(uint64(1)),
				ConnID:            new(uint64(42)),
				ConnAESKey:        new("0123456789abcdef"),
				KeepAliveInterval: new(int32(10)),
			},
		}
		body, _ := proto.Marshal(response)
		pkt, _ := codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if _, err := conn.Write(pkt); err != nil {
			return
		}

		notifyBody, _ := proto.Marshal(&notifypb.Response{
			RetType: new(int32(0)),
			S2C: &notifypb.S2C{
				Type: new(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
				ConnectStatus: &notifypb.ConnectStatus{
					QotLogined: new(true),
					TrdLogined: new(false),
				},
			},
		})
		notifyPacket, _ := codec.Encode(ProtoNotify, 0, notifyBody)
		_, _ = conn.Write(notifyPacket)
	}()
	defer func() { <-done }()

	c := New(Config{Addr: ln.Addr().String(), RequestTimeout: 3 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	notifyCh := make(chan *notifypb.Response, 1)
	c.SubscribeNotify(func(response *notifypb.Response) {
		select {
		case notifyCh <- response:
		default:
		}
	})

	var initResp initpb.Response
	if err := c.Call(ctx, ProtoInitConnect, &initpb.Request{C2S: &initpb.C2S{
		ClientVer:  new(int32(101)),
		ClientID:   new("jftrade-test"),
		RecvNotify: new(true),
	}}, &initResp); err != nil {
		t.Fatalf("init call: %v", err)
	}

	select {
	case recvNotify := <-recvNotifyCh:
		if !recvNotify {
			t.Fatal("expected InitConnect recvNotify=true")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for init request")
	}

	select {
	case response := <-notifyCh:
		if response.GetRetType() != 0 {
			t.Fatalf("unexpected notify retType: %d", response.GetRetType())
		}
		if response.GetS2C().GetType() != int32(notifypb.NotifyType_NotifyType_ConnStatus) {
			t.Fatalf("unexpected notify type: %d", response.GetS2C().GetType())
		}
		if !response.GetS2C().GetConnectStatus().GetQotLogined() || response.GetS2C().GetConnectStatus().GetTrdLogined() {
			t.Fatalf("unexpected connect status: %+v", response.GetS2C().GetConnectStatus())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for notify push")
	}
}

func TestCallIgnoresMismatchedProtoOnSameSerial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	notifyReceived := make(chan *notifypb.Response, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		header := make([]byte, codec.HeaderLen)
		for requestIndex := 0; ; requestIndex++ {
			if _, err := io.ReadFull(conn, header); err != nil {
				return
			}
			bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
			packet := make([]byte, codec.HeaderLen+bodyLen)
			copy(packet, header)
			if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
				return
			}
			frame, err := codec.Decode(packet)
			if err != nil {
				return
			}

			response := &initpb.Response{
				RetType: new(int32(0)),
				S2C: &initpb.S2C{
					ServerVer:         new(int32(700)),
					LoginUserID:       new(uint64(1)),
					ConnID:            new(uint64(42)),
					ConnAESKey:        new("0123456789abcdef"),
					KeepAliveInterval: new(int32(10)),
				},
			}

			if requestIndex == 1 {
				notifyBody, _ := proto.Marshal(&notifypb.Response{
					RetType: new(int32(0)),
					S2C: &notifypb.S2C{
						Type: new(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
						ConnectStatus: &notifypb.ConnectStatus{
							QotLogined: new(true),
							TrdLogined: new(false),
						},
					},
				})
				notifyPacket, _ := codec.Encode(ProtoNotify, frame.Header.SerialNo, notifyBody)
				if _, err := conn.Write(notifyPacket); err != nil {
					return
				}
			}

			body, _ := proto.Marshal(response)
			pkt, _ := codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
			if _, err := conn.Write(pkt); err != nil {
				return
			}
		}
	}()
	defer func() { <-done }()

	c := New(Config{Addr: ln.Addr().String(), RequestTimeout: 3 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	c.SubscribeNotify(func(response *notifypb.Response) {
		select {
		case notifyReceived <- response:
		default:
		}
	})

	request := &initpb.Request{C2S: &initpb.C2S{
		ClientVer: new(int32(101)),
		ClientID:  new("jftrade-test"),
	}}
	var initResp initpb.Response
	if err := c.Call(ctx, ProtoInitConnect, request, &initResp); err != nil {
		t.Fatalf("first init call: %v", err)
	}

	var secondResp initpb.Response
	if err := c.Call(ctx, ProtoInitConnect, request, &secondResp); err != nil {
		t.Fatalf("second init call with mismatched proto frame: %v", err)
	}
	if secondResp.GetS2C().GetConnID() != 42 {
		t.Fatalf("unexpected second response: %+v", &secondResp)
	}

	select {
	case response := <-notifyReceived:
		if response.GetS2C().GetType() != int32(notifypb.NotifyType_NotifyType_ConnStatus) {
			t.Fatalf("unexpected notify type: %d", response.GetS2C().GetType())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for mismatched proto push to be dispatched")
	}
}
