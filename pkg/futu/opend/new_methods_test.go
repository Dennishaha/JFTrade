package opend

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	getglobalstatepb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
	qotrequesthistoryklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"
	tradeunlockpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdunlocktrade"
)

// protoHandler is a function that builds a proto.Message response for a given
// decoded frame. Return nil response to not send a reply (push-like proto).
type protoHandler func(reqFrame codec.Frame) (proto.Message, error)

// fakeOpenD is a configurable fake OpenD TCP server used across all tests.
type fakeOpenD struct {
	addr   string
	ln     net.Listener
	mux    map[uint32]protoHandler // protoID -> handler
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	pushCh chan []byte // queued pushes to send
}

func newFakeOpenD(t *testing.T, addr string) *fakeOpenD {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeOpenD{
		addr:   ln.Addr().String(),
		ln:     ln,
		mux:    make(map[uint32]protoHandler),
		ctx:    ctx,
		cancel: cancel,
		pushCh: make(chan []byte, 16),
	}
	return f
}

// handle registers a protoHandler for a specific protoID.
func (f *fakeOpenD) handle(protoID uint32, fn protoHandler) {
	f.mux[protoID] = fn
}

// push queues an unsolicited packet to be sent on the next connection read.
func (f *fakeOpenD) push(pkt []byte) {
	select {
	case f.pushCh <- pkt:
	default:
	}
}

// serve starts the accept loop in a goroutine, handling one connection.
func (f *fakeOpenD) serve(t *testing.T) {
	t.Helper()
	f.wg.Add(1)
	go func() {
		func() {
			defer f.wg.Done()
			conn, err := f.ln.Accept()
			if err != nil {
				return
			}
			defer func() { jftradeCheckTestError(t, conn.Close()) }()
			for {
				select {
				case <-f.ctx.Done():
					return
				default:
				}
				// Flush any queued pushes before reading new requests.
				for {
					select {
					case pkt := <-f.pushCh:
						_, jftradeErr2 := conn.Write(pkt)
						jftradeCheckTestError(t, jftradeErr2)
					default:
						goto donePush
					}
				}
			donePush:

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
				handler, ok := f.mux[frame.Header.ProtoID]
				if !ok {
					// Default: respond with empty InitConnect
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
					body, jftradeErr4 := proto.Marshal(resp)
					jftradeCheckTestError(t, jftradeErr4)
					pkt, jftradeErr6 := codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
					jftradeCheckTestError(t, jftradeErr6)
					_, jftradeErr3 := conn.Write(pkt)
					jftradeCheckTestError(t, jftradeErr3)
					continue
				}
				respMsg, err := handler(frame)
				if err != nil {
					return
				}
				if respMsg == nil {
					continue // push-like, no response
				}
				// Some fake responses intentionally omit required protocol fields; the
				// partial payload is still useful for exercising client error paths.
				body, _ := proto.Marshal(respMsg) //nolint:errcheck // Intentionally malformed fake responses exercise client error paths.
				pkt, jftradeErr7 := codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
				jftradeCheckTestError(t, jftradeErr7)
				_, jftradeErr1 := conn.Write(pkt)
				jftradeCheckTestError(t, jftradeErr1)
			}
		}()
	}()
}

func (f *fakeOpenD) close() {
	f.cancel()
	jftradeErr1 := f.ln.Close()
	jftradePanicOnError(jftradeErr1)
	f.wg.Wait()
}

// --- Proto message helpers to fill required fields ---

func hkSecurity(code string) *qotcommonpb.Security {
	return &qotcommonpb.Security{Market: new(int32(1)), Code: new(code)}
}

func validBasicQot(security *qotcommonpb.Security, curPrice float64) *qotcommonpb.BasicQot {
	return &qotcommonpb.BasicQot{
		Security:       security,
		IsSuspended:    new(false),
		ListTime:       new("2000-01-01"),
		PriceSpread:    new(0.1),
		UpdateTime:     new("2026-05-31 16:00:00"),
		HighPrice:      new(curPrice * 1.02),
		OpenPrice:      new(curPrice * 0.99),
		LowPrice:       new(curPrice * 0.98),
		CurPrice:       new(curPrice),
		LastClosePrice: new(curPrice * 0.995),
		Volume:         new(int64(10000000)),
		Turnover:       new(curPrice * 10000000),
		TurnoverRate:   new(0.1),
		Amplitude:      new(4.0),
	}
}

func validKLine(timeStr string, open, close, high, low float64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       new(timeStr),
		IsBlank:    new(false),
		OpenPrice:  new(open),
		ClosePrice: new(close),
		HighPrice:  new(high),
		LowPrice:   new(low),
		Volume:     new(int64(1000000)),
		Turnover:   new(open * 1000000),
	}
}

func validSnapshotBasic(security *qotcommonpb.Security, name string, typ int32, price float64) *qotgetsecuritysnapshotpb.SnapshotBasicData {
	return &qotgetsecuritysnapshotpb.SnapshotBasicData{
		Security:       security,
		Name:           new(name),
		Type:           new(typ),
		IsSuspend:      new(false),
		ListTime:       new("2000-01-01"),
		LotSize:        new(int32(100)),
		PriceSpread:    new(0.1),
		UpdateTime:     new("2026-05-31 16:00:00"),
		HighPrice:      new(price * 1.02),
		OpenPrice:      new(price * 0.99),
		LowPrice:       new(price * 0.98),
		LastClosePrice: new(price * 0.995),
		CurPrice:       new(price),
		Volume:         new(int64(10000000)),
		Turnover:       new(price * 10000000),
		TurnoverRate:   new(0.1),
	}
}

// clientWithServer creates a connected client against a fakeOpenD with
// the given handler map, registers default handlers for ProtoInitConnect,
// and returns the client+server for cleanup.
func clientWithServer(t *testing.T, handlers map[uint32]protoHandler) (*Client, *fakeOpenD, context.Context) {
	t.Helper()
	server := newFakeOpenD(t, "127.0.0.1:0")
	for id, fn := range handlers {
		server.handle(id, fn)
	}
	// Default InitConnect handler so the client can initialize.
	server.handle(ProtoInitConnect, func(frame codec.Frame) (proto.Message, error) {
		return &initpb.Response{
			RetType: new(int32(0)),
			S2C: &initpb.S2C{
				ServerVer:         new(int32(700)),
				LoginUserID:       new(uint64(1)),
				ConnID:            new(uint64(42)),
				ConnAESKey:        new("0123456789abcdef"),
				KeepAliveInterval: new(int32(10)),
			},
		}, nil
	})
	server.serve(t)

	c := New(Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(func() {
		cancel()
		jftradeCheckTestError(t, c.Close())
		server.close()
	})
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Perform InitConnect to set connID
	var initResp initpb.Response
	if err := c.Call(ctx, ProtoInitConnect, &initpb.Request{C2S: &initpb.C2S{
		ClientVer: new(int32(101)),
		ClientID:  new("test"),
	}}, &initResp); err != nil {
		t.Fatalf("init connect: %v", err)
	}
	c.SetConnID(initResp.GetS2C().GetConnID())
	return c, server, ctx
}

// --- Test: GetGlobalState ---

func TestGetGlobalState(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetGlobalState: func(frame codec.Frame) (proto.Message, error) {
			return &getglobalstatepb.Response{
				RetType: new(int32(0)),
				S2C: &getglobalstatepb.S2C{
					MarketHK:       new(int32(qotcommonpb.QotMarketState_QotMarketState_Morning)),
					MarketUS:       new(int32(qotcommonpb.QotMarketState_QotMarketState_PreMarketBegin)),
					MarketSH:       new(int32(qotcommonpb.QotMarketState_QotMarketState_Closed)),
					MarketSZ:       new(int32(qotcommonpb.QotMarketState_QotMarketState_Closed)),
					MarketHKFuture: new(int32(qotcommonpb.QotMarketState_QotMarketState_WaitingOpen)),
					QotLogined:     new(true),
					TrdLogined:     new(true),
					ServerVer:      new(int32(900)),
					ServerBuildNo:  new(int32(5008)),
					Time:           new(int64(1717000000)),
				},
			}, nil
		},
	})

	gs, err := c.GetGlobalState(ctx)
	if err != nil {
		t.Fatalf("GetGlobalState: %v", err)
	}
	if gs.ServerVer != 900 {
		t.Fatalf("expected serverVer 900, got %d", gs.ServerVer)
	}
	if !gs.QotLogined {
		t.Fatal("expected qotLogined=true")
	}
	if !gs.TrdLogined {
		t.Fatal("expected trdLogined=true")
	}
	if gs.MarketHK != int32(qotcommonpb.QotMarketState_QotMarketState_Morning) {
		t.Fatalf("unexpected marketHK: %d", gs.MarketHK)
	}
}

// --- Test: SubscribeQuotes ---

func TestSubscribeQuotes(t *testing.T) {
	receivedSub := make(chan bool, 1)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoQotSub: func(frame codec.Frame) (proto.Message, error) {
			var req qotsubpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			c2s := req.GetC2S()
			if c2s == nil {
				return nil, nil
			}
			if c2s.GetIsSubOrUnSub() {
				receivedSub <- true
			}
			return &qotsubpb.Response{RetType: new(int32(0))}, nil
		},
	})

	err := c.SubscribeQuotes(ctx, QuoteSubRequest{
		Securities:  []*qotcommonpb.Security{{Market: new(int32(1)), Code: new("00700")}},
		SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_Basic},
		IsSubscribe: true,
	})
	if err != nil {
		t.Fatalf("SubscribeQuotes: %v", err)
	}
	select {
	case sub := <-receivedSub:
		if !sub {
			t.Fatal("expected IsSubOrUnSub=true")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for subscription")
	}
}

// --- Test: UnsubscribeQuotes ---

func TestUnsubscribeQuotes(t *testing.T) {
	receivedUnsub := make(chan bool, 1)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoQotSub: func(frame codec.Frame) (proto.Message, error) {
			var req qotsubpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			c2s := req.GetC2S()
			if c2s == nil {
				return nil, nil
			}
			if !c2s.GetIsSubOrUnSub() {
				receivedUnsub <- true
			}
			return &qotsubpb.Response{RetType: new(int32(0))}, nil
		},
	})

	err := c.SubscribeQuotes(ctx, QuoteSubRequest{
		Securities:  []*qotcommonpb.Security{{Market: new(int32(1)), Code: new("00700")}},
		SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_Basic},
		IsSubscribe: false,
	})
	if err != nil {
		t.Fatalf("SubscribeQuotes (unsub): %v", err)
	}
	select {
	case unsub := <-receivedUnsub:
		if !unsub {
			t.Fatal("expected IsSubOrUnSub=false")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for unsubscription")
	}
}

// --- Test: GetBasicQot ---

func TestGetBasicQot(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetBasicQot: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetbasicqotpb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetbasicqotpb.S2C{
					BasicQotList: []*qotcommonpb.BasicQot{validBasicQot(hkSecurity("00700"), 380.0)},
				},
			}, nil
		},
	})

	qots, err := c.GetBasicQot(ctx, []*qotcommonpb.Security{
		{Market: new(int32(1)), Code: new("00700")},
	})
	if err != nil {
		t.Fatalf("GetBasicQot: %v", err)
	}
	if len(qots) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(qots))
	}
	if qots[0].GetCurPrice() != 380.0 {
		t.Fatalf("expected curPrice 380.0, got %f", qots[0].GetCurPrice())
	}
}

// --- Test: SubscribeBasicQot ---

func TestSubscribeBasicQot(t *testing.T) {
	c, server, ctx := clientWithServer(t, map[uint32]protoHandler{
		// Add a dummy handler so we can trigger a read cycle
		ProtoGetGlobalState: func(frame codec.Frame) (proto.Message, error) {
			return &getglobalstatepb.Response{RetType: new(int32(0)), S2C: &getglobalstatepb.S2C{ServerVer: new(int32(1))}}, nil
		},
	})
	receivedCh := make(chan []*qotcommonpb.BasicQot, 1)
	c.SubscribeBasicQot(func(qots []*qotcommonpb.BasicQot) {
		select {
		case receivedCh <- qots:
		default:
		}
	})

	// Queue a push frame then send a request to wake the server's read loop
	body, jftradeErr2 := proto.Marshal(&qotupdatebasicqotpb.Response{
		RetType: new(int32(0)),
		S2C: &qotupdatebasicqotpb.S2C{
			BasicQotList: []*qotcommonpb.BasicQot{validBasicQot(hkSecurity("00700"), 382.5)},
		},
	})
	jftradeCheckTestError(t, jftradeErr2)
	pkt, jftradeErr3 := codec.Encode(ProtoQotUpdateBasicQot, 0, body)
	jftradeCheckTestError(t, jftradeErr3)
	server.push(pkt)

	// Issue a request to trigger the server read loop to flush the push
	// This request only wakes the server read loop; its intentionally incomplete
	// fake response is unrelated to the push assertion below.
	_, _ = c.GetGlobalState(ctx) //nolint:errcheck // This request only wakes the fake server so it can flush the push.

	select {
	case qots := <-receivedCh:
		if len(qots) != 1 {
			t.Fatalf("expected 1 push quote, got %d", len(qots))
		}
		if qots[0].GetCurPrice() != 382.5 {
			t.Fatalf("expected curPrice 382.5, got %f", qots[0].GetCurPrice())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for push quote")
	}
}

// --- Test: GetKL ---

func TestGetKL(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetKL: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetklpb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetklpb.S2C{
					Security: hkSecurity("00700"),
					Name:     new("Tencent"),
					KlList:   []*qotcommonpb.KLine{validKLine("2026-05-31 15:55", 378.0, 380.5, 381.0, 377.0)},
				},
			}, nil
		},
	})

	result, err := c.GetKL(ctx, KLineRequest{
		Security:  &qotcommonpb.Security{Market: new(int32(1)), Code: new("00700")},
		RehabType: qotcommonpb.RehabType_RehabType_Forward,
		KLType:    qotcommonpb.KLType_KLType_5Min,
		ReqNum:    10,
	})
	if err != nil {
		t.Fatalf("GetKL: %v", err)
	}
	if result.Name != "Tencent" {
		t.Fatalf("expected name Tencent, got %q", result.Name)
	}
	if len(result.KLines) != 1 {
		t.Fatalf("expected 1 kline, got %d", len(result.KLines))
	}
}

// --- Test: RequestHistoryKL ---

func TestRequestHistoryKL(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoRequestHistoryKL: func(frame codec.Frame) (proto.Message, error) {
			return &qotrequesthistoryklpb.Response{
				RetType: new(int32(0)),
				S2C: &qotrequesthistoryklpb.S2C{
					Security:   hkSecurity("00700"),
					Name:       new("Tencent"),
					NextReqKey: []byte("next_page_key"),
					KlList:     []*qotcommonpb.KLine{validKLine("2026-05-30", 370.0, 378.0, 380.0, 368.0)},
				},
			}, nil
		},
	})

	result, err := c.RequestHistoryKL(ctx, HistoryKLineRequest{
		Security:  &qotcommonpb.Security{Market: new(int32(1)), Code: new("00700")},
		RehabType: qotcommonpb.RehabType_RehabType_None,
		KLType:    qotcommonpb.KLType_KLType_Day,
		BeginTime: "2026-05-01",
		EndTime:   "2026-05-31",
	})
	if err != nil {
		t.Fatalf("RequestHistoryKL: %v", err)
	}
	if string(result.NextReqKey) != "next_page_key" {
		t.Fatalf("expected nextReqKey, got %q", string(result.NextReqKey))
	}
	if len(result.KLines) != 1 {
		t.Fatalf("expected 1 kline, got %d", len(result.KLines))
	}
}

// --- Test: GetStaticInfo ---

func TestGetStaticInfo(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetStaticInfo: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetstaticinfopb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetstaticinfopb.S2C{
					StaticInfoList: []*qotcommonpb.SecurityStaticInfo{
						{
							Basic: &qotcommonpb.SecurityStaticBasic{
								Security:      hkSecurity("00700"),
								Id:            new(int64(700)),
								Name:          new("Tencent"),
								SecType:       new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
								ListTime:      new("2004-06-16"),
								LotSize:       new(int32(100)),
								ListTimestamp: new(float64(1087324800)),
								Delisting:     new(false),
							},
						},
					},
				},
			}, nil
		},
	})

	info, err := c.GetStaticInfo(ctx, []*qotcommonpb.Security{
		{Market: new(int32(1)), Code: new("00700")},
	})
	if err != nil {
		t.Fatalf("GetStaticInfo: %v", err)
	}
	if len(info) != 1 {
		t.Fatalf("expected 1 static info, got %d", len(info))
	}
	if info[0].GetBasic().GetName() != "Tencent" {
		t.Fatalf("expected name Tencent, got %q", info[0].GetBasic().GetName())
	}
	if info[0].GetBasic().GetLotSize() != 100 {
		t.Fatalf("expected lotSize 100, got %d", info[0].GetBasic().GetLotSize())
	}
}

// --- Test: GetSecuritySnapshot ---

func TestGetSecuritySnapshot(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetSecuritySnapshot: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetsecuritysnapshotpb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetsecuritysnapshotpb.S2C{
					SnapshotList: []*qotgetsecuritysnapshotpb.Snapshot{
						{
							Basic: validSnapshotBasic(hkSecurity("00700"), "Tencent", int32(qotcommonpb.SecurityType_SecurityType_Eqty), 380.0),
							EquityExData: &qotgetsecuritysnapshotpb.EquitySnapshotExData{
								IssuedShares:         new(int64(9600000000)),
								IssuedMarketVal:      new(float64(3648000000000)),
								NetAsset:             new(float64(869000000000)),
								NetProfit:            new(float64(177000000000)),
								EarningsPershare:     new(18.5),
								OutstandingShares:    new(int64(9500000000)),
								OutstandingMarketVal: new(float64(3610000000000)),
								NetAssetPershare:     new(90.5),
								EyRate:               new(4.9),
								PeRate:               new(20.5),
								PbRate:               new(4.2),
								PeTTMRate:            new(18.3),
							},
						},
					},
				},
			}, nil
		},
	})

	snapshots, err := c.GetSecuritySnapshot(ctx, []*qotcommonpb.Security{
		{Market: new(int32(1)), Code: new("00700")},
	})
	if err != nil {
		t.Fatalf("GetSecuritySnapshot: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Basic.GetName() != "Tencent" {
		t.Fatalf("expected name Tencent, got %q", snapshots[0].Basic.GetName())
	}
	if snapshots[0].EquityExData.GetPeRate() != 20.5 {
		t.Fatalf("expected peRate 20.5, got %f", snapshots[0].EquityExData.GetPeRate())
	}
}

// --- Test: UnlockTrade ---

func TestUnlockTrade(t *testing.T) {
	receivedUnlock := make(chan bool, 1)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdUnlockTrade: func(frame codec.Frame) (proto.Message, error) {
			var req tradeunlockpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			receivedUnlock <- req.GetC2S().GetUnlock()
			return &tradeunlockpb.Response{RetType: new(int32(0))}, nil
		},
	})

	err := c.UnlockTrade(ctx, true, "dummyMD5", nil)
	if err != nil {
		t.Fatalf("UnlockTrade: %v", err)
	}
	select {
	case unlock := <-receivedUnlock:
		if !unlock {
			t.Fatal("expected unlock=true")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for unlock")
	}
}

func TestLockTrade(t *testing.T) {
	receivedUnlock := make(chan bool, 1)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdUnlockTrade: func(frame codec.Frame) (proto.Message, error) {
			var req tradeunlockpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			receivedUnlock <- req.GetC2S().GetUnlock()
			return &tradeunlockpb.Response{RetType: new(int32(0))}, nil
		},
	})

	err := c.UnlockTrade(ctx, false, "", nil)
	if err != nil {
		t.Fatalf("UnlockTrade (lock): %v", err)
	}
	select {
	case unlock := <-receivedUnlock:
		if unlock {
			t.Fatal("expected unlock=false")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for lock")
	}
}

// --- Test: Error responses ---

func TestGetBasicQotError(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetBasicQot: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetbasicqotpb.Response{
				RetType: new(int32(-1)),
				ErrCode: new(int32(1000)),
				RetMsg:  new("no permission"),
			}, nil
		},
	})

	_, err := c.GetBasicQot(ctx, []*qotcommonpb.Security{hkSecurity("00700")})
	if err == nil {
		t.Fatal("expected error for negative retType")
	}
}

func TestGetGlobalStateError(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetGlobalState: func(frame codec.Frame) (proto.Message, error) {
			return &getglobalstatepb.Response{
				RetType: new(int32(-1)),
				RetMsg:  new("server busy"),
			}, nil
		},
	})

	_, err := c.GetGlobalState(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSubscribeQuotesError(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoQotSub: func(frame codec.Frame) (proto.Message, error) {
			return &qotsubpb.Response{
				RetType: new(int32(-1)),
				RetMsg:  new("invalid security"),
			}, nil
		},
	})

	err := c.SubscribeQuotes(ctx, QuoteSubRequest{
		IsSubscribe: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnlockTradeError(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdUnlockTrade: func(frame codec.Frame) (proto.Message, error) {
			return &tradeunlockpb.Response{
				RetType: new(int32(-1)),
				RetMsg:  new("wrong password"),
			}, nil
		},
	})

	err := c.UnlockTrade(ctx, true, "wrong_pwd", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Test: Empty S2C ---

func TestGetBasicQotEmptyS2C(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetBasicQot: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetbasicqotpb.Response{RetType: new(int32(0))}, nil
		},
	})

	qots, err := c.GetBasicQot(ctx, []*qotcommonpb.Security{hkSecurity("00700")})
	if err != nil {
		t.Fatalf("GetBasicQot: %v", err)
	}
	if len(qots) != 0 {
		t.Fatalf("expected empty list for nil s2c, got %d", len(qots))
	}
}

func TestGetKLNullS2C(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetKL: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetklpb.Response{RetType: new(int32(0)), S2C: &qotgetklpb.S2C{Security: hkSecurity("00700")}}, nil
		},
	})

	result, err := c.GetKL(ctx, KLineRequest{
		Security:  &qotcommonpb.Security{Market: new(int32(1)), Code: new("00700")},
		RehabType: qotcommonpb.RehabType_RehabType_None,
		KLType:    qotcommonpb.KLType_KLType_Day,
		ReqNum:    10,
	})
	if err != nil {
		t.Fatalf("GetKL: %v", err)
	}
	if len(result.KLines) != 0 {
		t.Fatalf("expected 0 klines, got %d", len(result.KLines))
	}
}

// --- Test: GlobalState with advanced fields ---

func TestGetGlobalStateAdvancedFields(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetGlobalState: func(frame codec.Frame) (proto.Message, error) {
			return &getglobalstatepb.Response{
				RetType: new(int32(0)),
				S2C: &getglobalstatepb.S2C{
					MarketHK:       new(int32(3)),
					MarketUS:       new(int32(3)),
					MarketSH:       new(int32(6)),
					MarketSZ:       new(int32(6)),
					MarketHKFuture: new(int32(23)),
					MarketUSFuture: new(int32(23)),
					MarketSGFuture: new(int32(6)),
					MarketJPFuture: new(int32(6)),
					QotLogined:     new(true),
					TrdLogined:     new(true),
					ServerVer:      new(int32(1000)),
					ServerBuildNo:  new(int32(6000)),
					Time:           new(int64(1717000000)),
					LocalTime:      new(1717000000.0),
					ProgramStatus: &commonpb.ProgramStatus{
						Type: commonpb.ProgramStatusType_ProgramStatusType_Ready.Enum(),
					},
					QotSvrIpAddr: new("10.0.0.1"),
					TrdSvrIpAddr: new("10.0.0.2"),
					ConnID:       new(uint64(42)),
				},
			}, nil
		},
	})

	gs, err := c.GetGlobalState(ctx)
	if err != nil {
		t.Fatalf("GetGlobalState: %v", err)
	}
	if gs.ServerVer != 1000 {
		t.Fatalf("expected serverVer 1000")
	}
	if gs.QotSvrIPAddr == nil || *gs.QotSvrIPAddr != "10.0.0.1" {
		t.Fatalf("expected qotSvrIpAddr 10.0.0.1")
	}
	if gs.TrdSvrIPAddr == nil || *gs.TrdSvrIPAddr != "10.0.0.2" {
		t.Fatalf("expected trdSvrIpAddr 10.0.0.2")
	}
	if gs.ConnID == nil || *gs.ConnID != 42 {
		t.Fatalf("expected connID 42")
	}
	if gs.MarketUSFuture == nil || *gs.MarketUSFuture != 23 {
		t.Fatalf("expected marketUSFuture 23")
	}
	if gs.ProgramStatus == nil {
		t.Fatal("expected programStatus")
	}
}

// --- Test: HistoryKLine with pagination ---

func TestRequestHistoryKLPagination(t *testing.T) {
	maxAck := int32(100)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoRequestHistoryKL: func(frame codec.Frame) (proto.Message, error) {
			var req qotrequesthistoryklpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			// If nextReqKey is set, return final page
			if len(req.GetC2S().GetNextReqKey()) > 0 {
				return &qotrequesthistoryklpb.Response{
					RetType: new(int32(0)),
					S2C: &qotrequesthistoryklpb.S2C{
						Security: req.GetC2S().Security,
						KlList:   []*qotcommonpb.KLine{validKLine("2026-05-25", 365.0, 370.0, 372.0, 364.0)},
					},
				}, nil
			}
			return &qotrequesthistoryklpb.Response{
				RetType: new(int32(0)),
				S2C: &qotrequesthistoryklpb.S2C{
					Security:   req.GetC2S().Security,
					NextReqKey: []byte("page2"),
					KlList:     []*qotcommonpb.KLine{validKLine("2026-05-22", 375.0, 380.0, 382.0, 374.0)},
				},
			}, nil
		},
	})

	// First page
	result, err := c.RequestHistoryKL(ctx, HistoryKLineRequest{
		Security:    &qotcommonpb.Security{Market: new(int32(1)), Code: new("00700")},
		RehabType:   qotcommonpb.RehabType_RehabType_None,
		KLType:      qotcommonpb.KLType_KLType_Day,
		BeginTime:   "2026-05-01",
		EndTime:     "2026-05-31",
		MaxAckKLNum: &maxAck,
	})
	if err != nil {
		t.Fatalf("RequestHistoryKL (page1): %v", err)
	}
	if string(result.NextReqKey) != "page2" {
		t.Fatalf("expected nextReqKey 'page2', got %q", string(result.NextReqKey))
	}
	if len(result.KLines) != 1 {
		t.Fatalf("expected 1 kline on page1, got %d", len(result.KLines))
	}

	// Second page
	result, err = c.RequestHistoryKL(ctx, HistoryKLineRequest{
		Security:    &qotcommonpb.Security{Market: new(int32(1)), Code: new("00700")},
		RehabType:   qotcommonpb.RehabType_RehabType_None,
		KLType:      qotcommonpb.KLType_KLType_Day,
		BeginTime:   "2026-05-01",
		EndTime:     "2026-05-31",
		NextReqKey:  []byte("page2"),
		MaxAckKLNum: &maxAck,
	})
	if err != nil {
		t.Fatalf("RequestHistoryKL (page2): %v", err)
	}
	if len(result.NextReqKey) != 0 {
		t.Fatalf("expected empty nextReqKey on last page, got %q", string(result.NextReqKey))
	}
	if len(result.KLines) != 1 {
		t.Fatalf("expected 1 kline on page2, got %d", len(result.KLines))
	}
}

// --- Test: Multiple securities in GetBasicQot ---

func TestGetBasicQotMultipleSecurities(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetBasicQot: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetbasicqotpb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetbasicqotpb.S2C{
					BasicQotList: []*qotcommonpb.BasicQot{
						validBasicQot(hkSecurity("00700"), 380.0),
						validBasicQot(hkSecurity("09988"), 88.5),
					},
				},
			}, nil
		},
	})

	qots, err := c.GetBasicQot(ctx, []*qotcommonpb.Security{
		{Market: new(int32(1)), Code: new("00700")},
		{Market: new(int32(1)), Code: new("09988")},
	})
	if err != nil {
		t.Fatalf("GetBasicQot: %v", err)
	}
	if len(qots) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(qots))
	}
	if qots[0].GetCurPrice() != 380.0 {
		t.Fatalf("expected HK.00700 380.0, got %f", qots[0].GetCurPrice())
	}
	if qots[1].GetCurPrice() != 88.5 {
		t.Fatalf("expected HK.09988 88.5, got %f", qots[1].GetCurPrice())
	}
}

// --- Test: UnlockTrade with securityFirm ---

func TestUnlockTradeWithSecurityFirm(t *testing.T) {
	receivedFirm := make(chan int32, 1)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdUnlockTrade: func(frame codec.Frame) (proto.Message, error) {
			var req tradeunlockpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			if req.GetC2S().SecurityFirm != nil {
				receivedFirm <- req.GetC2S().GetSecurityFirm()
			}
			return &tradeunlockpb.Response{RetType: new(int32(0))}, nil
		},
	})

	firm := int32(1) // FutuSecurities
	err := c.UnlockTrade(ctx, true, "pwdMD5", &firm)
	if err != nil {
		t.Fatalf("UnlockTrade: %v", err)
	}
	select {
	case f := <-receivedFirm:
		if f != 1 {
			t.Fatalf("expected securityFirm 1, got %d", f)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for unlock with securityFirm")
	}
}

// --- Test: GetSecuritySnapshot with index type ---

func TestGetSecuritySnapshotIndex(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetSecuritySnapshot: func(frame codec.Frame) (proto.Message, error) {
			return &qotgetsecuritysnapshotpb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetsecuritysnapshotpb.S2C{
					SnapshotList: []*qotgetsecuritysnapshotpb.Snapshot{
						{
							Basic: validSnapshotBasic(hkSecurity("HSI"), "Hang Seng Index", int32(qotcommonpb.SecurityType_SecurityType_Index), 21000.0),
							IndexExData: &qotgetsecuritysnapshotpb.IndexSnapshotExData{
								RaiseCount: new(int32(35)),
								FallCount:  new(int32(25)),
								EqualCount: new(int32(20)),
							},
						},
					},
				},
			}, nil
		},
	})

	snapshots, err := c.GetSecuritySnapshot(ctx, []*qotcommonpb.Security{
		{Market: new(int32(1)), Code: new("HSI")},
	})
	if err != nil {
		t.Fatalf("GetSecuritySnapshot: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].IndexExData.GetRaiseCount() != 35 {
		t.Fatalf("expected raiseCount 35, got %d", snapshots[0].IndexExData.GetRaiseCount())
	}
}

// --- Test: SubscribeQuotes with all options ---

func TestSubscribeQuotesAllOptions(t *testing.T) {
	received := make(chan *qotsubpb.C2S, 1)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoQotSub: func(frame codec.Frame) (proto.Message, error) {
			var req qotsubpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			received <- req.GetC2S()
			return &qotsubpb.Response{RetType: new(int32(0))}, nil
		},
	})

	err := c.SubscribeQuotes(ctx, QuoteSubRequest{
		Securities: []*qotcommonpb.Security{
			{Market: new(int32(11)), Code: new("AAPL")},
		},
		SubTypes:          []qotcommonpb.SubType{qotcommonpb.SubType_SubType_Basic, qotcommonpb.SubType_SubType_KL_Day},
		IsSubscribe:       true,
		IsRegPush:         new(true),
		RegPushRehabTypes: []qotcommonpb.RehabType{qotcommonpb.RehabType_RehabType_Forward},
		IsFirstPush:       new(true),
		ExtendedTime:      new(true),
		Session:           new(int32(1)),
	})
	if err != nil {
		t.Fatalf("SubscribeQuotes: %v", err)
	}

	select {
	case c2s := <-received:
		if !c2s.GetIsSubOrUnSub() {
			t.Fatal("expected IsSubOrUnSub=true")
		}
		if !c2s.GetIsRegOrUnRegPush() {
			t.Fatal("expected IsRegOrUnRegPush=true")
		}
		if !c2s.GetIsFirstPush() {
			t.Fatal("expected IsFirstPush=true")
		}
		if !c2s.GetExtendedTime() {
			t.Fatal("expected ExtendedTime=true")
		}
		if c2s.GetSession() != 1 {
			t.Fatal("expected session=1")
		}
		if len(c2s.GetSecurityList()) != 1 {
			t.Fatalf("expected 1 security, got %d", len(c2s.GetSecurityList()))
		}
		if len(c2s.GetSubTypeList()) != 2 {
			t.Fatalf("expected 2 subTypes, got %d", len(c2s.GetSubTypeList()))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for subscription")
	}
}

// --- Test: SubscribeQuotes unsubAll ---

func TestSubscribeQuotesUnsubAll(t *testing.T) {
	received := make(chan *qotsubpb.C2S, 1)
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoQotSub: func(frame codec.Frame) (proto.Message, error) {
			var req qotsubpb.Request
			if err := proto.Unmarshal(frame.Body, &req); err != nil {
				return nil, err
			}
			received <- req.GetC2S()
			return &qotsubpb.Response{RetType: new(int32(0))}, nil
		},
	})

	err := c.SubscribeQuotes(ctx, QuoteSubRequest{
		IsUnsubAll: true,
	})
	if err != nil {
		t.Fatalf("SubscribeQuotes (unsubAll): %v", err)
	}

	select {
	case c2s := <-received:
		if !c2s.GetIsUnsubAll() {
			t.Fatal("expected IsUnsubAll=true")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for unsubAll")
	}
}

func jftradePanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
