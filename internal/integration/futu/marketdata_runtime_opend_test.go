package futu

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetsubinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsubinfo"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

type marketDataRuntimeOpenDServer struct {
	listener net.Listener
	empty    atomic.Bool
	zero     atomic.Bool
	wg       sync.WaitGroup
}

func startMarketDataRuntimeOpenDServer(t *testing.T) *marketDataRuntimeOpenDServer {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &marketDataRuntimeOpenDServer{listener: listener}
	server.wg.Go(func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			server.wg.Go(func() {
				server.handle(conn)
			})
		}
	})
	t.Cleanup(func() {
		_ = listener.Close()
		server.wg.Wait()
	})
	return server
}

func (s *marketDataRuntimeOpenDServer) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
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
		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}

		var response proto.Message
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			response = &initpb.Response{RetType: new(int32(0)), S2C: &initpb.S2C{
				ServerVer: new(int32(1008)), LoginUserID: new(uint64(1)), ConnID: new(uint64(42)),
				ConnAESKey: new("0123456789abcdef"), KeepAliveInterval: new(int32(10)),
			}}
		case opend.ProtoGetGlobalState:
			zero := int32(0)
			response = &globalpb.Response{RetType: new(int32(0)), S2C: &globalpb.S2C{
				MarketHK: &zero, MarketUS: &zero, MarketSH: &zero, MarketSZ: &zero,
				MarketHKFuture: &zero, QotLogined: new(true), TrdLogined: new(true),
				ServerVer: new(int32(1008)), ServerBuildNo: new(int32(6808)), Time: new(int64(0)),
			}}
		case opend.ProtoGetBasicQot:
			var request qotgetbasicqotpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return
			}
			quotes := []*qotcommonpb.BasicQot{}
			if !s.empty.Load() {
				for _, security := range request.GetC2S().GetSecurityList() {
					currentPrice := 190.5
					openPrice := 189.0
					if s.zero.Load() {
						currentPrice = 0
						openPrice = 0
					}
					quotes = append(quotes, &qotcommonpb.BasicQot{
						Security: security, IsSuspended: new(false), ListTime: new("2020-01-01"),
						PriceSpread: new(0.01), CurPrice: new(currentPrice), OpenPrice: new(openPrice),
						HighPrice: new(191.0), LowPrice: new(188.5), LastClosePrice: new(188.0),
						Volume: new(int64(1234)), Turnover: new(235077.0),
						TurnoverRate: new(1.25), Amplitude: new(2.5),
						UpdateTime: new("2026-07-15 09:31:00"), UpdateTimestamp: new(float64(1773567060)),
					})
				}
			}
			response = &qotgetbasicqotpb.Response{RetType: new(int32(0)), S2C: &qotgetbasicqotpb.S2C{BasicQotList: quotes}}
		case opend.ProtoQotSub:
			response = &qotsubpb.Response{RetType: new(int32(0))}
		case opend.ProtoGetSubInfo:
			response = &qotgetsubinfopb.Response{RetType: new(int32(0)), S2C: &qotgetsubinfopb.S2C{
				TotalUsedQuota: new(int32(1)), RemainQuota: new(int32(99)),
			}}
		default:
			return
		}
		body, err := proto.Marshal(response)
		if err != nil {
			return
		}
		packet, err = codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if err != nil {
			return
		}
		if _, err := conn.Write(packet); err != nil {
			return
		}
	}
}

func TestMarketDataRuntimeQueryAndSubscriptionWrappers(t *testing.T) {
	server := startMarketDataRuntimeOpenDServer(t)
	host, portText, err := net.SplitHostPort(server.listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	now := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	runtime := NewMarketDataRuntime(MarketDataRuntimeOptions{
		ConfigSource: func() MarketDataConfig {
			return MarketDataConfig{Enabled: true, Host: host, APIPort: port}
		},
		Now: func() time.Time { return now },
	})
	t.Cleanup(func() { _ = runtime.Close() })

	ticks, err := runtime.QueryTickers(context.Background(), []string{"US.AAPL"})
	if err != nil || ticks["US.AAPL"].InstrumentID != "US.AAPL" {
		t.Fatalf("QueryTickers = %#v, %v", ticks, err)
	}
	tick, err := runtime.QueryTicker(context.Background(), "US.AAPL")
	if err != nil || tick == nil || tick.Price.String() != "190.5" {
		t.Fatalf("QueryTicker = %#v, %v", tick, err)
	}
	snapshot, err := runtime.QuerySnapshot(context.Background(), "US.AAPL")
	if err != nil || snapshot == nil || snapshot.Price.String() != "190.5" {
		t.Fatalf("QuerySnapshot = %#v, %v", snapshot, err)
	}

	if _, err := runtime.QueryTickers(context.Background(), []string{"invalid"}); err == nil {
		t.Fatal("invalid QueryTickers error = nil")
	}
	server.zero.Store(true)
	if _, err := runtime.QueryTicker(context.Background(), "US.AAPL"); err == nil {
		t.Fatal("zero-price QueryTicker error = nil")
	}
	server.zero.Store(false)
	server.empty.Store(true)
	if _, err := runtime.QuerySnapshot(context.Background(), "US.AAPL"); err == nil {
		t.Fatal("empty QuerySnapshot error = nil")
	}
	server.empty.Store(false)

	desired := []marketdata.InstrumentRef{{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m"}}
	if err := runtime.ReconcileSubscriptions(context.Background(), desired); err != nil {
		t.Fatalf("ReconcileSubscriptions: %v", err)
	}
	if state := runtime.SubscriptionState(); state == nil || state["desiredCount"] != 1 {
		t.Fatalf("SubscriptionState = %#v", state)
	}

	var nilRuntime *MarketDataRuntime
	if err := nilRuntime.ReconcileSubscriptions(nil, nil); err != nil {
		t.Fatalf("nil ReconcileSubscriptions = %v", err)
	}
	if state := nilRuntime.SubscriptionState(); state != nil {
		t.Fatalf("nil SubscriptionState = %#v", state)
	}
}
