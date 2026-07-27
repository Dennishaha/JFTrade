package futu

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
)

func TestProbeOpenDReportsProtocolOutcomesWithoutARealOpenD(t *testing.T) {
	tests := []struct {
		name       string
		reply      func(codec.Frame) (proto.Message, bool)
		wantStatus string
		wantError  string
	}{
		{
			name: "connection closes during init",
			reply: func(codec.Frame) (proto.Message, bool) {
				return nil, false
			},
			wantStatus: "degraded",
			wantError:  "client closed",
		},
		{
			name: "init is rejected without a message",
			reply: func(frame codec.Frame) (proto.Message, bool) {
				if frame.Header.ProtoID != opend.ProtoInitConnect {
					return nil, false
				}
				return &initpb.Response{RetType: new(int32(commonpb.RetType_RetType_Failed))}, true
			},
			wantStatus: "degraded",
			wantError:  "InitConnect failed: retType=-1",
		},
		{
			name: "connection closes during global state",
			reply: func(frame codec.Frame) (proto.Message, bool) {
				if frame.Header.ProtoID == opend.ProtoInitConnect {
					return successfulProbeInitResponse(), true
				}
				return nil, false
			},
			wantStatus: "degraded",
			wantError:  "client closed",
		},
		{
			name: "global state is rejected without a message",
			reply: func(frame codec.Frame) (proto.Message, bool) {
				switch frame.Header.ProtoID {
				case opend.ProtoInitConnect:
					return successfulProbeInitResponse(), true
				case opend.ProtoGetGlobalState:
					return &globalpb.Response{RetType: new(int32(commonpb.RetType_RetType_Failed))}, true
				default:
					return nil, false
				}
			},
			wantStatus: "degraded",
			wantError:  "GetGlobalState failed: retType=-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := startProbeProtocolServer(t, tt.reply)
			probe := ProbeOpenD(t.Context(), ProbeConfig{Host: host, APIPort: port})
			if probe.Status != tt.wantStatus {
				t.Fatalf("probe status = %q, want %q: %#v", probe.Status, tt.wantStatus, probe)
			}
			if tt.wantError == "" {
				if probe.Connectivity != "connected" || probe.LastError != nil || probe.ServerVersion == nil {
					t.Fatalf("healthy probe = %#v", probe)
				}
				return
			}
			if probe.Connectivity != "degraded" || probe.LastError == nil || !strings.Contains(*probe.LastError, tt.wantError) {
				t.Fatalf("degraded probe = %#v, want error containing %q", probe, tt.wantError)
			}
		})
	}
}

func TestProbeOpenDMapsHealthyProtocolFixture(t *testing.T) {
	server := startMarketDataRuntimeOpenDServer(t)
	host, portText, err := net.SplitHostPort(server.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	probe := ProbeOpenD(t.Context(), ProbeConfig{Host: host, APIPort: port})
	if probe.Status != "healthy" || probe.Connectivity != "connected" ||
		probe.LastError != nil || probe.ServerVersion == nil || len(probe.Markets) != 4 {
		t.Fatalf("healthy protocol probe = %#v", probe)
	}
}

func TestProbeOpenDReportsClosedPortAsDisconnected(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	probe := ProbeOpenD(t.Context(), ProbeConfig{Host: host, APIPort: port})
	if probe.Status != "offline" || probe.Connectivity != "disconnected" || probe.LastError == nil {
		t.Fatalf("closed-port probe = %#v", probe)
	}
}

func TestProbeFromGlobalStateEnforcesMinimumVersionAndMapsNeutralState(t *testing.T) {
	if probe := ProbeFromGlobalState("checked-nil", nil); probe.Status != "degraded" || probe.LastError == nil {
		t.Fatalf("nil state probe = %#v", probe)
	}

	old := ProbeFromGlobalState("checked-old", &globalpb.S2C{
		ServerVer: new(int32(1008)), ServerBuildNo: new(int32(6708)),
	})
	if old.IssueCode != "OPEND_VERSION_UNSUPPORTED" || old.Status != "degraded" || old.LastError == nil ||
		old.ServerVersion == nil || *old.ServerVersion != "10.8.6708" {
		t.Fatalf("unsupported version probe = %#v", old)
	}

	description := "all services ready"
	statusType := commonpb.ProgramStatusType_ProgramStatusType_Ready
	healthy := ProbeFromGlobalState("checked-healthy", &globalpb.S2C{
		QotLogined: new(true), TrdLogined: new(false),
		ServerVer: new(int32(1009)), ServerBuildNo: new(int32(6908)),
		Time:     new(int64(1_700_000_000)),
		MarketHK: new(int32(1)), MarketUS: new(int32(2)),
		MarketSH: new(int32(3)), MarketSZ: new(int32(4)),
		ProgramStatus: &commonpb.ProgramStatus{Type: &statusType, StrExtDesc: &description},
	})
	if healthy.Status != "healthy" || healthy.Connectivity != "connected" ||
		healthy.QuoteLoggedIn == nil || !*healthy.QuoteLoggedIn ||
		healthy.TradeLoggedIn == nil || *healthy.TradeLoggedIn ||
		healthy.ProgramStatus == nil || *healthy.ProgramStatus != "ProgramStatusType_Ready: all services ready" ||
		healthy.ProgramTimestamp == nil || len(healthy.Markets) != 4 {
		t.Fatalf("healthy neutral probe = %#v", healthy)
	}
	if healthy.Markets[0].Market != "HK" || healthy.Markets[0].State != "1" ||
		healthy.Markets[3].Market != "SZ" || healthy.Markets[3].State != "4" {
		t.Fatalf("market state mapping = %#v", healthy.Markets)
	}
}

func TestProgramStatusStringHandlesMissingPlainAndDescribedStatus(t *testing.T) {
	if got := ProgramStatusString(nil); got != "Unavailable" {
		t.Fatalf("nil program status = %q", got)
	}
	statusType := commonpb.ProgramStatusType_ProgramStatusType_Loging
	if got := ProgramStatusString(&commonpb.ProgramStatus{Type: &statusType}); got != "ProgramStatusType_Loging" {
		t.Fatalf("plain program status = %q", got)
	}
	description := "waiting for credentials"
	if got := ProgramStatusString(&commonpb.ProgramStatus{Type: &statusType, StrExtDesc: &description}); got != "ProgramStatusType_Loging: waiting for credentials" {
		t.Fatalf("described program status = %q", got)
	}
}

func successfulProbeInitResponse() *initpb.Response {
	return &initpb.Response{RetType: new(int32(commonpb.RetType_RetType_Succeed))}
}

type probeProtocolServer struct {
	listener net.Listener
	done     chan struct{}
	stopOnce sync.Once
	reply    func(codec.Frame) (proto.Message, bool)
}

func startProbeProtocolServer(
	t *testing.T,
	reply func(codec.Frame) (proto.Message, bool),
) (string, int) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &probeProtocolServer{listener: listener, done: make(chan struct{}), reply: reply}
	go server.serve()
	t.Cleanup(server.stop)

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func (s *probeProtocolServer) serve() {
	defer close(s.done)
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		frame, err := readProbeProtocolFrame(conn)
		if err != nil {
			return
		}
		response, keepOpen := s.reply(frame)
		if response == nil || !keepOpen {
			return
		}
		body, err := proto.Marshal(response)
		if err != nil {
			return
		}
		packet, err := codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if err != nil {
			return
		}
		if _, err := conn.Write(packet); err != nil {
			return
		}
	}
}

func (s *probeProtocolServer) stop() {
	s.stopOnce.Do(func() {
		_ = s.listener.Close()
		select {
		case <-s.done:
		case <-time.After(time.Second):
		}
	})
}

func readProbeProtocolFrame(conn net.Conn) (codec.Frame, error) {
	header := make([]byte, codec.HeaderLen)
	if _, err := io.ReadFull(conn, header); err != nil {
		return codec.Frame{}, err
	}
	bodyLength := int(binary.LittleEndian.Uint32(header[12:16]))
	if bodyLength < 0 || bodyLength > codec.MaxFrameBodyLen {
		return codec.Frame{}, io.ErrUnexpectedEOF
	}
	packet := make([]byte, codec.HeaderLen+bodyLength)
	copy(packet, header)
	if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
		return codec.Frame{}, err
	}
	return codec.Decode(packet)
}
