package opend

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
	qotrequesthistoryklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"
	updateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
)

func TestSubscribeQuotesEncodesAdvancedMarketDataOptions(t *testing.T) {
	received := make(chan *qotsubpb.C2S, 1)
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoQotSub: func(frame codec.Frame) (proto.Message, error) {
			var request qotsubpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			received <- request.GetC2S()
			return &qotsubpb.Response{RetType: new(int32(0))}, nil
		},
	})

	regPush := false
	firstPush := true
	extendedTime := true
	session := int32(2)
	orderBookDetail := true
	err := client.SubscribeQuotes(ctx, QuoteSubRequest{
		Securities:           []*qotcommonpb.Security{hkSecurity("00700")},
		SubTypes:             []qotcommonpb.SubType{qotcommonpb.SubType_SubType_Basic, qotcommonpb.SubType_SubType_KL_Day},
		IsSubscribe:          true,
		IsRegPush:            &regPush,
		RegPushRehabTypes:    []qotcommonpb.RehabType{qotcommonpb.RehabType_RehabType_Forward, qotcommonpb.RehabType_RehabType_Backward},
		IsFirstPush:          &firstPush,
		IsUnsubAll:           true,
		ExtendedTime:         &extendedTime,
		Session:              &session,
		IsSubOrderBookDetail: &orderBookDetail,
	})
	if err != nil {
		t.Fatalf("SubscribeQuotes() error = %v", err)
	}

	select {
	case request := <-received:
		if request == nil || len(request.GetSecurityList()) != 1 || request.GetSecurityList()[0].GetCode() != "00700" {
			t.Fatalf("security list = %#v", request)
		}
		if got := request.GetSubTypeList(); len(got) != 2 || got[0] != int32(qotcommonpb.SubType_SubType_Basic) || got[1] != int32(qotcommonpb.SubType_SubType_KL_Day) {
			t.Fatalf("sub type list = %v", got)
		}
		if request.GetIsRegOrUnRegPush() || !request.GetIsFirstPush() || !request.GetIsUnsubAll() {
			t.Fatalf("push flags = reg:%t first:%t unsubAll:%t", request.GetIsRegOrUnRegPush(), request.GetIsFirstPush(), request.GetIsUnsubAll())
		}
		if got := request.GetRegPushRehabTypeList(); len(got) != 2 || got[0] != int32(qotcommonpb.RehabType_RehabType_Forward) || got[1] != int32(qotcommonpb.RehabType_RehabType_Backward) {
			t.Fatalf("rehab type list = %v", got)
		}
		if !request.GetExtendedTime() || request.GetSession() != session || !request.GetIsSubOrderBookDetail() {
			t.Fatalf("market options = extended:%t session:%d detail:%t", request.GetExtendedTime(), request.GetSession(), request.GetIsSubOrderBookDetail())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for quote subscription request")
	}
}

func TestRequestHistoryKLEncodesOptionalFieldsAndHandlesEmptyResult(t *testing.T) {
	received := make(chan *qotrequesthistoryklpb.C2S, 1)
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoRequestHistoryKL: func(frame codec.Frame) (proto.Message, error) {
			var request qotrequesthistoryklpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			received <- request.GetC2S()
			return &qotrequesthistoryklpb.Response{RetType: new(int32(0))}, nil
		},
	})

	maxAck := int32(250)
	needFields := int64(0x3f)
	extendedTime := true
	session := int32(1)
	result, err := client.RequestHistoryKL(ctx, HistoryKLineRequest{
		Security:     hkSecurity("00700"),
		RehabType:    qotcommonpb.RehabType_RehabType_Forward,
		KLType:       qotcommonpb.KLType_KLType_15Min,
		BeginTime:    "2026-06-01",
		EndTime:      "2026-06-30",
		MaxAckKLNum:  &maxAck,
		NeedKLFields: &needFields,
		NextReqKey:   []byte("page-2"),
		ExtendedTime: &extendedTime,
		Session:      &session,
	})
	if err != nil {
		t.Fatalf("RequestHistoryKL() error = %v", err)
	}
	if result == nil || result.Security != nil || result.Name != "" || len(result.KLines) != 0 || len(result.NextReqKey) != 0 {
		t.Fatalf("empty history result = %#v", result)
	}

	select {
	case request := <-received:
		if request == nil || request.GetSecurity().GetCode() != "00700" || request.GetBeginTime() != "2026-06-01" || request.GetEndTime() != "2026-06-30" {
			t.Fatalf("history request identity = %#v", request)
		}
		if request.GetMaxAckKLNum() != maxAck || request.GetNeedKLFieldsFlag() != needFields || string(request.GetNextReqKey()) != "page-2" {
			t.Fatalf("history request pagination = max:%d fields:%d key:%q", request.GetMaxAckKLNum(), request.GetNeedKLFieldsFlag(), request.GetNextReqKey())
		}
		if !request.GetExtendedTime() || request.GetSession() != session {
			t.Fatalf("history market options = extended:%t session:%d", request.GetExtendedTime(), request.GetSession())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for history request")
	}
}

func TestMarketReadMethodsPropagateOpenDBusinessErrors(t *testing.T) {
	tests := []struct {
		name     string
		protoID  uint32
		response proto.Message
		call     func(*Client) error
		want     string
	}{
		{
			name:     "current kline entitlement",
			protoID:  ProtoGetKL,
			response: &qotgetklpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(9)), RetMsg: new("kline entitlement denied")},
			call: func(client *Client) error {
				_, err := client.GetKL(t.Context(), KLineRequest{Security: hkSecurity("00700"), KLType: qotcommonpb.KLType_KLType_Day, ReqNum: 2})
				return err
			},
			want: "Qot_GetKL retType=-1 errCode=9 retMsg=kline entitlement denied",
		},
		{
			name:     "history rate limit",
			protoID:  ProtoRequestHistoryKL,
			response: &qotrequesthistoryklpb.Response{RetType: new(int32(-2)), ErrCode: new(int32(429)), RetMsg: new("history rate limited")},
			call: func(client *Client) error {
				_, err := client.RequestHistoryKL(t.Context(), HistoryKLineRequest{Security: hkSecurity("00700"), KLType: qotcommonpb.KLType_KLType_Day})
				return err
			},
			want: "Qot_RequestHistoryKL retType=-2 errCode=429 retMsg=history rate limited",
		},
		{
			name:     "static info invalid security",
			protoID:  ProtoGetStaticInfo,
			response: &qotgetstaticinfopb.Response{RetType: new(int32(-1)), ErrCode: new(int32(400)), RetMsg: new("unknown security")},
			call: func(client *Client) error {
				_, err := client.GetStaticInfo(t.Context(), []*qotcommonpb.Security{hkSecurity("bad")})
				return err
			},
			want: "Qot_GetStaticInfo retType=-1 errCode=400 retMsg=unknown security",
		},
		{
			name:     "snapshot entitlement",
			protoID:  ProtoGetSecuritySnapshot,
			response: &qotgetsecuritysnapshotpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(403)), RetMsg: new("snapshot entitlement denied")},
			call: func(client *Client) error {
				_, err := client.GetSecuritySnapshot(t.Context(), []*qotcommonpb.Security{hkSecurity("00700")})
				return err
			},
			want: "Qot_GetSecuritySnapshot retType=-1 errCode=403 retMsg=snapshot entitlement denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _, _ := clientWithServer(t, map[uint32]protoHandler{
				tt.protoID: func(codec.Frame) (proto.Message, error) { return tt.response, nil },
			})
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSecurityInfoMethodsReturnEmptyCollectionsForEmptyOpenDResults(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetStaticInfo: func(codec.Frame) (proto.Message, error) {
			return &qotgetstaticinfopb.Response{RetType: new(int32(0))}, nil
		},
		ProtoGetSecuritySnapshot: func(codec.Frame) (proto.Message, error) {
			return &qotgetsecuritysnapshotpb.Response{RetType: new(int32(0))}, nil
		},
	})

	staticInfo, err := client.GetStaticInfo(ctx, []*qotcommonpb.Security{hkSecurity("00700")})
	if err != nil || staticInfo == nil || len(staticInfo) != 0 {
		t.Fatalf("GetStaticInfo() = (%#v, %v), want non-nil empty slice", staticInfo, err)
	}
	snapshots, err := client.GetSecuritySnapshot(ctx, []*qotcommonpb.Security{hkSecurity("00700")})
	if err != nil || snapshots == nil || len(snapshots) != 0 {
		t.Fatalf("GetSecuritySnapshot() = (%#v, %v), want non-nil empty slice", snapshots, err)
	}
}

func TestGetKLReturnsEmptyResultWhenOpenDOmitsS2C(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetKL: func(codec.Frame) (proto.Message, error) {
			return &qotgetklpb.Response{RetType: new(int32(0))}, nil
		},
	})

	result, err := client.GetKL(ctx, KLineRequest{
		Security: hkSecurity("00700"),
		KLType:   qotcommonpb.KLType_KLType_Day,
		ReqNum:   1,
	})
	if err != nil {
		t.Fatalf("GetKL() error = %v", err)
	}
	if result == nil || result.Security != nil || result.Name != "" || len(result.KLines) != 0 {
		t.Fatalf("empty current kline result = %#v", result)
	}
}

func TestMarketReadMethodsRejectDisconnectedSession(t *testing.T) {
	client := New(Config{})
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "basic quote",
			call: func() error {
				_, err := client.GetBasicQot(t.Context(), []*qotcommonpb.Security{hkSecurity("00700")})
				return err
			},
		},
		{
			name: "current kline",
			call: func() error {
				_, err := client.GetKL(t.Context(), KLineRequest{Security: hkSecurity("00700"), KLType: qotcommonpb.KLType_KLType_Day, ReqNum: 1})
				return err
			},
		},
		{
			name: "history kline",
			call: func() error {
				_, err := client.RequestHistoryKL(t.Context(), HistoryKLineRequest{Security: hkSecurity("00700"), KLType: qotcommonpb.KLType_KLType_Day})
				return err
			},
		},
		{
			name: "static info",
			call: func() error {
				_, err := client.GetStaticInfo(t.Context(), []*qotcommonpb.Security{hkSecurity("00700")})
				return err
			},
		},
		{
			name: "security snapshot",
			call: func() error {
				_, err := client.GetSecuritySnapshot(t.Context(), []*qotcommonpb.Security{hkSecurity("00700")})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrClosed) {
				t.Fatalf("error = %v, want ErrClosed", err)
			}
		})
	}
}

func TestMarketPushSubscribersIgnoreMalformedAndUnsuccessfulUpdates(t *testing.T) {
	client := New(Config{})
	client.SubscribeBasicQot(nil)
	client.SubscribeOrderBook(nil)

	basicCalls := 0
	client.SubscribeBasicQot(func(qots []*qotcommonpb.BasicQot) {
		basicCalls++
		if len(qots) != 1 || qots[0].GetSecurity().GetCode() != "00700" {
			t.Fatalf("basic quote push = %#v", qots)
		}
	})
	orderBookCalls := 0
	client.SubscribeOrderBook(func(update *updateorderbookpb.S2C) {
		orderBookCalls++
		if update.GetSecurity().GetCode() != "00700" {
			t.Fatalf("order book push = %#v", update)
		}
	})

	dispatchPush := func(protoID uint32, message proto.Message) {
		t.Helper()
		body, err := proto.Marshal(message)
		if err != nil {
			t.Fatalf("marshal push: %v", err)
		}
		client.dispatch(codec.Frame{Header: codec.Header{ProtoID: protoID}, Body: body})
	}

	client.dispatch(codec.Frame{Header: codec.Header{ProtoID: ProtoQotUpdateBasicQot}, Body: []byte{0xff}})
	dispatchPush(ProtoQotUpdateBasicQot, &qotupdatebasicqotpb.Response{RetType: new(int32(-1)), RetMsg: new("stale subscription")})
	dispatchPush(ProtoQotUpdateBasicQot, &qotupdatebasicqotpb.Response{RetType: new(int32(0))})
	dispatchPush(ProtoQotUpdateBasicQot, &qotupdatebasicqotpb.Response{
		RetType: new(int32(0)),
		S2C:     &qotupdatebasicqotpb.S2C{BasicQotList: []*qotcommonpb.BasicQot{validBasicQot(hkSecurity("00700"), 380)}},
	})

	client.dispatch(codec.Frame{Header: codec.Header{ProtoID: ProtoQotUpdateOrderBook}, Body: []byte{0xff}})
	dispatchPush(ProtoQotUpdateOrderBook, &updateorderbookpb.Response{RetType: new(int32(-1)), RetMsg: new("order book push rejected")})
	dispatchPush(ProtoQotUpdateOrderBook, &updateorderbookpb.Response{RetType: new(int32(0))})
	dispatchPush(ProtoQotUpdateOrderBook, &updateorderbookpb.Response{
		RetType: new(int32(0)),
		S2C:     &updateorderbookpb.S2C{Security: hkSecurity("00700")},
	})

	if basicCalls != 1 || orderBookCalls != 1 {
		t.Fatalf("callback counts = basic:%d orderBook:%d, want 1 each", basicCalls, orderBookCalls)
	}
}
