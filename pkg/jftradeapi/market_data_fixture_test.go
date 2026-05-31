package jftradeapi

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

type marketDataQuoteOpenDServer struct {
	addr                  string
	listener              net.Listener
	stopOnce              sync.Once
	shutdownCompleted     chan struct{}
	basicQotCalls         atomic.Int32
	securitySnapshotCalls atomic.Int32
	staticInfoCalls       atomic.Int32
	historyMu             sync.Mutex
	historyPages          [][]*qotcommonpb.KLine
	historyPagesBySession map[int32][][]*qotcommonpb.KLine
	currentKLines         []*qotcommonpb.KLine
	currentKLCalls        atomic.Int32
}

func startMarketDataQuoteOpenDServer(t *testing.T) *marketDataQuoteOpenDServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &marketDataQuoteOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
	}
	go server.acceptLoop()
	return server
}

func (s *marketDataQuoteOpenDServer) stop() {
	s.stopOnce.Do(func() {
		_ = s.listener.Close()
		<-s.shutdownCompleted
	})
}

func (s *marketDataQuoteOpenDServer) basicQotCallCount() int {
	return int(s.basicQotCalls.Load())
}

func (s *marketDataQuoteOpenDServer) securitySnapshotCallCount() int {
	return int(s.securitySnapshotCalls.Load())
}

func (s *marketDataQuoteOpenDServer) staticInfoCallCount() int {
	return int(s.staticInfoCalls.Load())
}

func (s *marketDataQuoteOpenDServer) acceptLoop() {
	defer close(s.shutdownCompleted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *marketDataQuoteOpenDServer) handleConn(conn net.Conn) {
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
		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}

		var response proto.Message
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			response = &initpb.Response{
				RetType: proto.Int32(0),
				S2C: &initpb.S2C{
					ServerVer:         proto.Int32(700),
					LoginUserID:       proto.Uint64(1),
					ConnID:            proto.Uint64(42),
					ConnAESKey:        proto.String("0123456789abcdef"),
					KeepAliveInterval: proto.Int32(10),
				},
			}
		case opend.ProtoQotSub:
			response = &qotsubpb.Response{RetType: proto.Int32(0)}
		case opend.ProtoGetBasicQot:
			s.basicQotCalls.Add(1)
			response = s.basicQotResponse(frame.Body)
		case opend.ProtoGetSecuritySnapshot:
			s.securitySnapshotCalls.Add(1)
			response = s.securitySnapshotResponse(frame.Body)
		case opend.ProtoGetStaticInfo:
			s.staticInfoCalls.Add(1)
			response = s.staticInfoResponse(frame.Body)
		case opend.ProtoRequestHistoryKL:
			response = s.historyKLResponse(frame.Body)
		case opend.ProtoGetKL:
			s.currentKLCalls.Add(1)
			response = s.currentKLResponse(frame.Body)
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

func (s *marketDataQuoteOpenDServer) securitySnapshotResponse(body []byte) *qotgetsecuritysnapshotpb.Response {
	request := &qotgetsecuritysnapshotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetsecuritysnapshotpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	quoteAt := time.Now().UTC().Truncate(time.Second)
	snapshots := make([]*qotgetsecuritysnapshotpb.Snapshot, 0, len(request.GetC2S().GetSecurityList()))
	for _, security := range request.GetC2S().GetSecurityList() {
		snapshots = append(snapshots, marketDataSecuritySnapshotFixture(security, quoteAt))
	}

	return &qotgetsecuritysnapshotpb.Response{
		RetType: proto.Int32(0),
		S2C:     &qotgetsecuritysnapshotpb.S2C{SnapshotList: snapshots},
	}
}

func (s *marketDataQuoteOpenDServer) staticInfoResponse(body []byte) *qotgetstaticinfopb.Response {
	request := &qotgetstaticinfopb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetstaticinfopb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	entries := make([]*qotcommonpb.SecurityStaticInfo, 0, len(request.GetC2S().GetSecurityList()))
	for _, security := range request.GetC2S().GetSecurityList() {
		entries = append(entries, marketDataSecurityStaticInfoFixture(security))
	}

	return &qotgetstaticinfopb.Response{
		RetType: proto.Int32(0),
		S2C:     &qotgetstaticinfopb.S2C{StaticInfoList: entries},
	}
}

func (s *marketDataQuoteOpenDServer) basicQotResponse(body []byte) *qotgetbasicqotpb.Response {
	request := &qotgetbasicqotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetbasicqotpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	quotes := make([]*qotcommonpb.BasicQot, 0, len(request.GetC2S().GetSecurityList()))
	quoteAt := time.Now().UTC().Truncate(time.Second)
	for _, security := range request.GetC2S().GetSecurityList() {
		quotes = append(quotes, &qotcommonpb.BasicQot{
			Security:        security,
			IsSuspended:     proto.Bool(false),
			ListTime:        proto.String("2020-01-01"),
			PriceSpread:     proto.Float64(0.01),
			UpdateTime:      proto.String(quoteAt.Format("2006-01-02 15:04:05")),
			HighPrice:       proto.Float64(322.6),
			OpenPrice:       proto.Float64(319.8),
			LowPrice:        proto.Float64(319.6),
			CurPrice:        proto.Float64(321.4),
			LastClosePrice:  proto.Float64(318.9),
			Volume:          proto.Int64(1282100),
			Turnover:        proto.Float64(411020000),
			TurnoverRate:    proto.Float64(1.25),
			Amplitude:       proto.Float64(2.5),
			UpdateTimestamp: proto.Float64(float64(quoteAt.Unix())),
		})
	}

	return &qotgetbasicqotpb.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetbasicqotpb.S2C{
			BasicQotList: quotes,
		},
	}
}
