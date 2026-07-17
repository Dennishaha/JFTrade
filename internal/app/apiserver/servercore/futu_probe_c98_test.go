package servercore

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestCoverage98FutuRuntimeReturnsLiveDiscoveredAccounts(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}, {
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(2001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	defer opendServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, opendServer.addr)
	runtime := server.brokerRuntime(t.Context())
	session, ok := runtime["session"].(map[string]any)
	if !ok {
		t.Fatalf("broker runtime session = %#v", runtime)
	}
	if got := session["connectivity"]; got != "connected" {
		t.Fatalf("broker runtime connectivity = %v, want connected", got)
	}
	if got := session["accountsDiscovered"]; got != 2 {
		t.Fatalf("accountsDiscovered = %v, want 2", got)
	}
	accounts, ok := runtime["accounts"].([]any)
	if !ok || len(accounts) != 2 {
		t.Fatalf("discovered accounts = %#v", runtime["accounts"])
	}
	first, ok := accounts[0].(futu.RuntimeAccount)
	if !ok || first.AccountID != "2001" || first.TradingEnvironment != "REAL" {
		t.Fatalf("first normalized discovered account = %#v", accounts[0])
	}
}

func TestCoverage98FutuRuntimeFallsBackWhenConnectedProbeHasNoExchange(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	defer opendServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, opendServer.addr)
	if probe := server.probeOpenD(t.Context()); probe.Connectivity != "connected" {
		t.Fatalf("precondition healthy probe = %#v", probe)
	}
	marketdataRuntime := server.marketdataRuntime
	server.marketdataRuntime = nil
	t.Cleanup(func() { _ = marketdataRuntime.Close() })

	runtime := server.brokerRuntime(t.Context())
	session, ok := runtime["session"].(map[string]any)
	if !ok || session["connectivity"] != "disconnected" || session["accountsDiscovered"] != 0 {
		t.Fatalf("runtime without exchange must stay safely empty: %#v", runtime)
	}
	if accounts, ok := runtime["accounts"].([]any); !ok || len(accounts) != 0 {
		t.Fatalf("runtime without exchange accounts = %#v", runtime["accounts"])
	}
}

func TestCoverage98FutuOnboardingDoesNotReportMissingEnabledManagedAccount(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.CreateManagedAccount(ManagedBrokerAccount{
		BrokerID: "futu", AccountID: "sim-1001", TradingEnvironment: "SIMULATE", Market: "HK", Enabled: true,
	}); err != nil {
		t.Fatalf("CreateManagedAccount: %v", err)
	}
	server := newTestServer(t, settings)
	stubCoverage98HealthyNodeRuntime(t)

	state := server.onboardingStateFromSettings(t.Context(), OnboardingSettings{})
	reasons, ok := state["reasons"].([]map[string]any)
	if !ok {
		t.Fatalf("onboarding reasons = %#v", state["reasons"])
	}
	for _, reason := range reasons {
		if reason["code"] == "NO_MANAGED_ACCOUNTS" {
			t.Fatalf("enabled account was treated as missing: %#v", reasons)
		}
	}
}

func TestCoverage98FutuProbeReportsOpenDProtocolFailures(t *testing.T) {
	tests := []struct {
		name     string
		reply    func(codec.Frame) (proto.Message, bool)
		contains string
	}{
		{
			name: "connection closes during init",
			reply: func(codec.Frame) (proto.Message, bool) {
				return nil, false
			},
			contains: "client closed",
		},
		{
			name: "init response rejects without message",
			reply: func(frame codec.Frame) (proto.Message, bool) {
				if frame.Header.ProtoID != opend.ProtoInitConnect {
					return nil, false
				}
				return &initpb.Response{RetType: new(int32(commonpb.RetType_RetType_Failed))}, true
			},
			contains: "InitConnect failed: retType=-1",
		},
		{
			name: "connection closes during global state",
			reply: func(frame codec.Frame) (proto.Message, bool) {
				if frame.Header.ProtoID == opend.ProtoInitConnect {
					return coverage98InitConnectSuccess(), true
				}
				return nil, false
			},
			contains: "client closed",
		},
		{
			name: "global state rejects without message",
			reply: func(frame codec.Frame) (proto.Message, bool) {
				switch frame.Header.ProtoID {
				case opend.ProtoInitConnect:
					return coverage98InitConnectSuccess(), true
				case opend.ProtoGetGlobalState:
					return &globalpb.Response{RetType: new(int32(commonpb.RetType_RetType_Failed))}, true
				default:
					return nil, false
				}
			},
			contains: "GetGlobalState failed: retType=-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opendServer := startCoverage98ProbeOpenDServer(t, tt.reply)
			server := newMarketDataTestServerWithQuoteRuntime(t, opendServer.addr)

			probe := server.probeOpenD(t.Context())
			if probe.Connectivity != "degraded" || probe.Status != "degraded" || probe.LastError == nil {
				t.Fatalf("protocol failure probe = %#v", probe)
			}
			if got := *probe.LastError; !strings.Contains(got, tt.contains) {
				t.Fatalf("protocol failure message = %q, want %q", got, tt.contains)
			}
		})
	}
}

func stubCoverage98HealthyNodeRuntime(t *testing.T) {
	t.Helper()
	previousLookPath := runtimeDependencyLookPath
	previousOutput := runtimeDependencyOutput
	runtimeDependencyLookPath = func(string) (string, error) { return "/test/node", nil }
	runtimeDependencyOutput = func(context.Context, string, ...string) ([]byte, error) { return []byte("v22.0.0"), nil }
	t.Cleanup(func() {
		runtimeDependencyLookPath = previousLookPath
		runtimeDependencyOutput = previousOutput
	})
}

func coverage98InitConnectSuccess() *initpb.Response {
	return &initpb.Response{RetType: new(int32(commonpb.RetType_RetType_Succeed))}
}

type coverage98ProbeOpenDServer struct {
	addr     string
	listener net.Listener
	reply    func(codec.Frame) (proto.Message, bool)
	done     chan struct{}
	stopOnce sync.Once
}

func startCoverage98ProbeOpenDServer(t *testing.T, reply func(codec.Frame) (proto.Message, bool)) *coverage98ProbeOpenDServer {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &coverage98ProbeOpenDServer{
		addr: listener.Addr().String(), listener: listener, reply: reply, done: make(chan struct{}),
	}
	go server.acceptLoop()
	t.Cleanup(server.stop)
	return server
}

func (s *coverage98ProbeOpenDServer) stop() {
	s.stopOnce.Do(func() {
		_ = s.listener.Close()
		<-s.done
	})
}

func (s *coverage98ProbeOpenDServer) acceptLoop() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *coverage98ProbeOpenDServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for {
		frame, err := readCoverage98OpenDFrame(conn)
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

func readCoverage98OpenDFrame(conn net.Conn) (codec.Frame, error) {
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
