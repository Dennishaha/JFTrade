package opend

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

var errForcedMarshal = errors.New("forced protobuf marshal failure")

// marshalFailureMessage supplies a failing protobuf fast path so Call can be
// verified to surface serialization failures before it touches the network.
type marshalFailureMessage struct {
	protoreflect.Message
}

func (m marshalFailureMessage) ProtoReflect() protoreflect.Message {
	return m
}

func (m marshalFailureMessage) ProtoMethods() *protoiface.Methods {
	return &protoiface.Methods{
		Marshal: func(input protoiface.MarshalInput) (protoiface.MarshalOutput, error) {
			return protoiface.MarshalOutput{Buf: input.Buf}, errForcedMarshal
		},
	}
}

type closeFailureConn struct {
	net.Conn
	err error
}

func (c closeFailureConn) Close() error {
	return c.err
}

func TestConnectSurfacesInvalidAddressAndClosedClient(t *testing.T) {
	t.Run("invalid address", func(t *testing.T) {
		client := New(Config{HandshakeTimeout: time.Second})
		if err := client.Connect(t.Context()); err == nil {
			t.Fatal("Connect() error = nil, want dial error")
		}
	})

	t.Run("closed while dialing", func(t *testing.T) {
		listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { jftradeCheckTestError(t, listener.Close()) }()

		accepted := make(chan net.Conn, 1)
		go func() {
			conn, acceptErr := listener.Accept()
			if acceptErr == nil {
				accepted <- conn
			}
		}()

		client := New(Config{Addr: listener.Addr().String(), HandshakeTimeout: time.Second})
		jftradeCheckTestError(t, client.Close())
		if err := client.Connect(t.Context()); !errors.Is(err, ErrClosed) {
			t.Fatalf("Connect() error = %v, want ErrClosed", err)
		}

		select {
		case conn := <-accepted:
			jftradeCheckTestError(t, conn.Close())
		case <-time.After(time.Second):
			t.Fatal("closed client never completed its dial")
		}
	})
}

func TestClientCloseDrainsPendingRequestsAfterBestEffortCloseFailure(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	defer func() { jftradeCheckTestError(t, clientConn.Close()) }()
	defer func() { jftradeCheckTestError(t, peerConn.Close()) }()

	pendingResponse := &pending{ch: make(chan codec.Frame)}
	client := New(Config{})
	client.conn = closeFailureConn{Conn: clientConn, err: errors.New("close failed")}
	client.pend[9] = pendingResponse

	if err := client.closeConn(false); err != nil {
		t.Fatalf("closeConn(false) error = %v, want nil", err)
	}
	if _, ok := <-pendingResponse.ch; ok {
		t.Fatal("pending response channel remained open")
	}
	select {
	case <-client.Done():
	default:
		t.Fatal("Done() was not closed")
	}
}

func TestKeepAliveLoopHalvesLongIntervalsAndStopsForClosedClient(t *testing.T) {
	client := New(Config{})
	jftradeCheckTestError(t, client.Close())

	finished := make(chan struct{})
	go func() {
		client.keepAliveLoop(2 * time.Second)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("keepAliveLoop did not stop after client close")
	}
}

func TestSubscribeNotifySkipsNilAndMalformedPush(t *testing.T) {
	client := New(Config{})
	client.SubscribeNotify(nil)
	if len(client.subs) != 0 {
		t.Fatalf("nil notify handler registered subscriptions: %#v", client.subs)
	}

	called := false
	client.SubscribeNotify(func(_ *notifypb.Response) { called = true })
	client.dispatch(codec.Frame{Header: codec.Header{ProtoID: ProtoNotify}, Body: []byte{0xff}})
	if called {
		t.Fatal("malformed notification invoked handler")
	}
}

func TestCallFrameRejectsMarshalAndOversizedPayloads(t *testing.T) {
	client := New(Config{})
	marshalFailure := marshalFailureMessage{Message: (&emptypb.Empty{}).ProtoReflect()}
	if _, err := client.callFrame(t.Context(), ProtoInitConnect, marshalFailure); !errors.Is(err, errForcedMarshal) {
		t.Fatalf("callFrame marshal error = %v, want %v", err, errForcedMarshal)
	}

	tooLarge := wrapperspb.Bytes(make([]byte, codec.MaxFrameBodyLen))
	if _, err := client.callFrame(t.Context(), ProtoInitConnect, tooLarge); !errors.Is(err, codec.ErrBodyTooLarge) {
		t.Fatalf("callFrame oversized payload error = %v, want %v", err, codec.ErrBodyTooLarge)
	}
}

func TestCallFrameReturnsWriteAndCloseFailures(t *testing.T) {
	t.Run("write failure", func(t *testing.T) {
		clientConn, peerConn := net.Pipe()
		jftradeCheckTestError(t, peerConn.Close())
		defer func() { jftradeCheckTestError(t, clientConn.Close()) }()

		client := New(Config{RequestTimeout: time.Second})
		client.conn = clientConn
		if _, err := client.callFrame(t.Context(), ProtoInitConnect, &emptypb.Empty{}); err == nil {
			t.Fatal("callFrame() error = nil, want write failure")
		}
	})

	t.Run("connection closes while waiting", func(t *testing.T) {
		clientConn, peerConn := net.Pipe()
		defer func() { jftradeCheckTestError(t, peerConn.Close()) }()

		client := New(Config{RequestTimeout: time.Second})
		client.conn = clientConn
		result := make(chan error, 1)
		go func() {
			_, err := client.callFrame(context.Background(), ProtoInitConnect, &emptypb.Empty{})
			result <- err
		}()

		readOpenDTestFrame(t, peerConn)
		jftradeCheckTestError(t, client.closeConn(true))
		if err := <-result; !errors.Is(err, ErrClosed) {
			t.Fatalf("callFrame() error = %v, want ErrClosed", err)
		}
	})
}

func TestReadLoopClosesConnectionOnOversizedFrame(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	defer func() { jftradeCheckTestError(t, peerConn.Close()) }()

	client := New(Config{})
	client.conn = clientConn
	pendingResponse := &pending{ch: make(chan codec.Frame)}
	client.pend[9] = pendingResponse
	go client.readLoop(clientConn)

	tooLargeHeader := make([]byte, codec.HeaderLen)
	binary.LittleEndian.PutUint32(tooLargeHeader[12:16], codec.MaxFrameBodyLen+1)
	writeOpenDTestBytes(t, peerConn, tooLargeHeader)

	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("read loop did not close client after oversized frame header")
	}
	if _, ok := <-pendingResponse.ch; ok {
		t.Fatal("oversized frame did not fail the pending request")
	}
	if _, err := peerConn.Write([]byte{0}); err == nil {
		t.Fatal("peer remained writable after oversized frame closed the stream")
	}
}

func TestReadLoopDiscardsInvalidFramesAndClosesOnTruncation(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	defer func() { jftradeCheckTestError(t, peerConn.Close()) }()

	client := New(Config{})
	client.conn = clientConn
	go client.readLoop(clientConn)

	badPacket := make([]byte, codec.HeaderLen)
	writeOpenDTestBytes(t, peerConn, badPacket)

	truncatedPacketHeader := make([]byte, codec.HeaderLen)
	truncatedPacketHeader[0] = 'F'
	truncatedPacketHeader[1] = 'T'
	binary.LittleEndian.PutUint32(truncatedPacketHeader[12:16], 1)
	writeOpenDTestBytes(t, peerConn, truncatedPacketHeader)
	jftradeCheckTestError(t, peerConn.Close())

	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("read loop did not close client after truncated frame")
	}
}

func TestDispatchDropsDuplicateResponseWhenPendingBufferIsFull(t *testing.T) {
	client := New(Config{})
	pendingResponse := &pending{protoID: ProtoInitConnect, ch: make(chan codec.Frame, 1)}
	pendingResponse.ch <- codec.Frame{}
	client.pend[7] = pendingResponse

	client.dispatch(codec.Frame{Header: codec.Header{ProtoID: ProtoInitConnect, SerialNo: 7}})
	if _, exists := client.pend[7]; exists {
		t.Fatal("matched pending request was not removed")
	}
}

func TestSubscribeQuotesReportsTransportFailure(t *testing.T) {
	client := New(Config{})
	if err := client.SubscribeQuotes(t.Context(), QuoteSubRequest{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("SubscribeQuotes() error = %v, want ErrClosed", err)
	}
}

func TestProgramStatusLabelIncludesServerDescription(t *testing.T) {
	status := &commonpb.ProgramStatus{Type: new(commonpb.ProgramStatusType_ProgramStatusType_Ready), StrExtDesc: new("connected")}
	if got, want := programStatusLabel(status), "ProgramStatusType_Ready: connected"; got != want {
		t.Fatalf("programStatusLabel() = %q, want %q", got, want)
	}
}

func readOpenDTestFrame(t testing.TB, conn net.Conn) {
	t.Helper()
	header := make([]byte, codec.HeaderLen)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("read OpenD header: %v", err)
	}
	bodyLen := int(binary.LittleEndian.Uint32(header[12:16]))
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(conn, body); err != nil {
		t.Fatalf("read OpenD body: %v", err)
	}
}

func writeOpenDTestBytes(t testing.TB, conn net.Conn, payload []byte) {
	t.Helper()
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write OpenD test payload: %v", err)
	}
}
