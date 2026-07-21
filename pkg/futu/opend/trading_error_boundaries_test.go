package opend

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetacclistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetacclist"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgethistoryorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderfilllist"
	trdgethistoryorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderlist"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
	trdgetorderfeepb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfee"
	trdgetorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfilllist"
	trdgetorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderlist"
	trdgetpositionlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetpositionlist"
)

func TestTradingReadMethodsPropagateOpenDBusinessErrors(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdGetAccList: func(codec.Frame) (proto.Message, error) {
			return &trdgetacclistpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1101)), RetMsg: new("account unavailable")}, nil
		},
		ProtoTrdGetFunds: func(codec.Frame) (proto.Message, error) {
			return &trdgetfundspb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1102)), RetMsg: new("funds unavailable")}, nil
		},
		ProtoTrdGetPositionList: func(codec.Frame) (proto.Message, error) {
			return &trdgetpositionlistpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1103)), RetMsg: new("positions unavailable")}, nil
		},
		ProtoTrdGetOrderList: func(codec.Frame) (proto.Message, error) {
			return &trdgetorderlistpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1104)), RetMsg: new("orders unavailable")}, nil
		},
		ProtoTrdGetHistoryOrderList: func(codec.Frame) (proto.Message, error) {
			return &trdgethistoryorderlistpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1105)), RetMsg: new("history orders unavailable")}, nil
		},
		ProtoTrdGetHistoryOrderFillList: func(codec.Frame) (proto.Message, error) {
			return &trdgethistoryorderfilllistpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1106)), RetMsg: new("history fills unavailable")}, nil
		},
		ProtoTrdGetOrderFee: func(codec.Frame) (proto.Message, error) {
			return &trdgetorderfeepb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1107)), RetMsg: new("fees unavailable")}, nil
		},
		ProtoTrdGetOrderFillList: func(codec.Frame) (proto.Message, error) {
			return &trdgetorderfilllistpb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1108)), RetMsg: new("fills unavailable")}, nil
		},
		ProtoTrdGetMarginRatio: func(codec.Frame) (proto.Message, error) {
			return &trdgetmarginratiopb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1109)), RetMsg: new("margin unavailable")}, nil
		},
		ProtoTrdFlowSummary: func(codec.Frame) (proto.Message, error) {
			return &trdflowsummarypb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1110)), RetMsg: new("flow unavailable")}, nil
		},
		ProtoTrdGetMaxTrdQtys: func(codec.Frame) (proto.Message, error) {
			return &trdgetmaxtrdqtyspb.Response{RetType: new(int32(-1)), ErrCode: new(int32(1111)), RetMsg: new("quantity unavailable")}, nil
		},
	})

	header := testTrdHeader(8080)
	maxQtyRequest := testMaxTrdQtyRequest(header)
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"account list", func() error { _, err := client.GetAccountList(ctx); return err }, "Trd_GetAccList retType=-1 errCode=1101 retMsg=account unavailable"},
		{"funds", func() error { _, err := client.GetFunds(ctx, header, trdcommonpb.Currency_Currency_HKD); return err }, "Trd_GetFunds retType=-1 errCode=1102 retMsg=funds unavailable"},
		{"positions", func() error { _, err := client.GetPositionList(ctx, header, nil); return err }, "Trd_GetPositionList retType=-1 errCode=1103 retMsg=positions unavailable"},
		{"orders", func() error { _, err := client.GetOrderList(ctx, header, nil); return err }, "Trd_GetOrderList retType=-1 errCode=1104 retMsg=orders unavailable"},
		{"history orders", func() error {
			_, err := client.GetHistoryOrderList(ctx, header, &trdcommonpb.TrdFilterConditions{}, nil)
			return err
		}, "Trd_GetHistoryOrderList retType=-1 errCode=1105 retMsg=history orders unavailable"},
		{"history fills", func() error {
			_, err := client.GetHistoryOrderFillList(ctx, header, &trdcommonpb.TrdFilterConditions{})
			return err
		}, "Trd_GetHistoryOrderFillList retType=-1 errCode=1106 retMsg=history fills unavailable"},
		{"order fees", func() error { _, err := client.GetOrderFee(ctx, header, []string{"order-1"}); return err }, "Trd_GetOrderFee retType=-1 errCode=1107 retMsg=fees unavailable"},
		{"fills", func() error { _, err := client.GetOrderFillList(ctx, header, nil); return err }, "Trd_GetOrderFillList retType=-1 errCode=1108 retMsg=fills unavailable"},
		{"margin ratios", func() error {
			_, err := client.GetMarginRatio(ctx, header, []*qotcommonpb.Security{hkSecurity("00700")})
			return err
		}, "Trd_GetMarginRatio retType=-1 errCode=1109 retMsg=margin unavailable"},
		{"cash flow", func() error { _, err := client.GetFlowSummary(ctx, header, "2026-07-01", nil); return err }, "Trd_FlowSummary retType=-1 errCode=1110 retMsg=flow unavailable"},
		{"maximum quantity", func() error { _, err := client.GetMaxTrdQtys(ctx, maxQtyRequest); return err }, "Trd_GetMaxTrdQtys retType=-1 errCode=1111 retMsg=quantity unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestTradingReadMethodsRejectDisconnectedSession(t *testing.T) {
	client := New(Config{})
	header := testTrdHeader(8080)
	maxQtyRequest := testMaxTrdQtyRequest(header)
	tests := []struct {
		name string
		call func() error
	}{
		{"account list", func() error { _, err := client.GetAccountList(t.Context()); return err }},
		{"funds", func() error {
			_, err := client.GetFunds(t.Context(), header, trdcommonpb.Currency_Currency_HKD)
			return err
		}},
		{"positions", func() error { _, err := client.GetPositionList(t.Context(), header, nil); return err }},
		{"orders", func() error { _, err := client.GetOrderList(t.Context(), header, nil); return err }},
		{"history orders", func() error { _, err := client.GetHistoryOrderList(t.Context(), header, nil, nil); return err }},
		{"history fills", func() error { _, err := client.GetHistoryOrderFillList(t.Context(), header, nil); return err }},
		{"order fees", func() error { _, err := client.GetOrderFee(t.Context(), header, []string{"order-1"}); return err }},
		{"fills", func() error { _, err := client.GetOrderFillList(t.Context(), header, nil); return err }},
		{"margin ratios", func() error {
			_, err := client.GetMarginRatio(t.Context(), header, []*qotcommonpb.Security{hkSecurity("00700")})
			return err
		}},
		{"cash flow", func() error { _, err := client.GetFlowSummary(t.Context(), header, "2026-07-01", nil); return err }},
		{"maximum quantity", func() error { _, err := client.GetMaxTrdQtys(t.Context(), maxQtyRequest); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrClosed) {
				t.Fatalf("error = %v, want ErrClosed", err)
			}
		})
	}
}

func TestHistoryTradingReadsReturnStableEmptyCollections(t *testing.T) {
	header := testTrdHeader(8080)
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdGetHistoryOrderList: func(codec.Frame) (proto.Message, error) {
			return &trdgethistoryorderlistpb.Response{RetType: new(int32(0))}, nil
		},
		ProtoTrdGetHistoryOrderFillList: func(codec.Frame) (proto.Message, error) {
			return &trdgethistoryorderfilllistpb.Response{RetType: new(int32(0))}, nil
		},
	})

	orders, err := client.GetHistoryOrderList(ctx, header, nil, nil)
	if err != nil || orders == nil || len(orders) != 0 {
		t.Fatalf("GetHistoryOrderList() = (%#v, %v), want non-nil empty slice", orders, err)
	}
	fills, err := client.GetHistoryOrderFillList(ctx, header, nil)
	if err != nil || fills == nil || len(fills) != 0 {
		t.Fatalf("GetHistoryOrderFillList() = (%#v, %v), want non-nil empty slice", fills, err)
	}
}

func testMaxTrdQtyRequest(header *trdcommonpb.TrdHeader) *trdgetmaxtrdqtyspb.C2S {
	return &trdgetmaxtrdqtyspb.C2S{
		Header:    header,
		OrderType: new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		Code:      new("00700"),
		Price:     new(320.5),
	}
}
