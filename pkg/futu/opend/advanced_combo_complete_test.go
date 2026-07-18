package opend

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	eventcategorypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgeteventcontractcategory"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetcombomaxpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetcombomaxtrdqtys"
	trdplacecombopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplacecomboorder"
)

func TestAdvancedDispatcherKeysValidationSuccessAndFailures(t *testing.T) {
	keys := AdvancedProtocolKeys()
	if len(keys) != len(AdvancedProtocols) || keys[0] >= keys[len(keys)-1] {
		t.Fatalf("advanced keys = %#v", keys)
	}
	if AdvancedC2SHasField("missing", "field") ||
		AdvancedC2SHasField("Qot_GetEventContractCategory", "missing") {
		t.Fatal("unknown advanced C2S field was reported present")
	}
	if err := ValidateAdvancedC2S("missing", nil); err == nil {
		t.Fatal("unknown advanced protocol validated")
	}
	if err := ValidateAdvancedC2S(
		"Qot_GetEventContractCategory",
		map[string]any{"unsupported": true},
	); err == nil {
		t.Fatal("unknown advanced request field validated")
	}
	if err := ValidateAdvancedC2S(
		"Qot_GetEventContractCategory",
		map[string]any{"category": make(chan int)},
	); err == nil {
		t.Fatal("unmarshalable advanced request validated")
	}
	if err := ValidateAdvancedC2S(
		"Qot_GetEventContractCategory",
		map[string]any{"category": "SPORTS"},
	); err != nil {
		t.Fatalf("valid advanced request: %v", err)
	}
	if _, err := newRegisteredMessage("Missing.Message"); err == nil {
		t.Fatal("missing registered message was created")
	}
}

func TestCallAdvancedSuccessEnvelopeAndAllErrorBoundaries(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		3434: func(codec.Frame) (proto.Message, error) {
			return &eventcategorypb.Response{
				RetType: new(int32(0)),
				S2C: &eventcategorypb.S2C{CategoryList: []*eventcategorypb.CategoryItem{{
					Category: new("SPORTS"),
				}}},
			}, nil
		},
	})
	payload, err := c.CallAdvanced(ctx, "Qot_GetEventContractCategory", map[string]any{})
	if err != nil {
		t.Fatalf("CallAdvanced success: %v", err)
	}
	if len(payload) == 0 {
		t.Fatalf("advanced payload = %#v", payload)
	}
	if _, err := c.CallAdvanced(ctx, "missing", nil); err == nil {
		t.Fatal("unknown CallAdvanced protocol succeeded")
	}
	if _, err := c.CallAdvanced(ctx, "Qot_GetEventContractCategory", map[string]any{
		"category": make(chan int),
	}); err == nil {
		t.Fatal("unmarshalable CallAdvanced request succeeded")
	}
	if _, err := c.CallAdvanced(ctx, "Qot_GetEventContractCategory", map[string]any{
		"unsupported": true,
	}); err == nil {
		t.Fatal("strict CallAdvanced request succeeded")
	}

	errorClient, _, errorCtx := clientWithServer(t, map[uint32]protoHandler{
		3434: func(codec.Frame) (proto.Message, error) {
			return &eventcategorypb.Response{
				RetType: new(int32(1)), ErrCode: new(int32(429)), RetMsg: new("limited"),
			}, nil
		},
	})
	if _, err := errorClient.CallAdvanced(
		errorCtx, "Qot_GetEventContractCategory", map[string]any{},
	); err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("advanced response error = %v", err)
	}

	closed := New(Config{})
	if _, err := closed.CallAdvanced(
		t.Context(), "Qot_GetEventContractCategory", map[string]any{},
	); err == nil {
		t.Fatal("closed advanced client call succeeded")
	}
}

func TestAdvancedResponseValidationAndPayloadHelpers(t *testing.T) {
	if err := validateAdvancedResponse("bad", &initpb.C2S{}); err == nil {
		t.Fatal("response without retType validated")
	}
	if err := validateAdvancedResponse("ok", &eventcategorypb.Response{
		RetType: new(int32(0)),
	}); err != nil {
		t.Fatalf("successful response validation: %v", err)
	}
	if err := validateAdvancedResponse("failed", &eventcategorypb.Response{
		RetType: new(int32(1)),
	}); err == nil {
		t.Fatal("failed response without optional details validated")
	}
	payload, err := advancedResponsePayload("empty", &eventcategorypb.Response{
		RetType: new(int32(0)),
	})
	if err != nil || len(payload) != 0 {
		t.Fatalf("empty advanced payload = %#v, %v", payload, err)
	}
	if _, err := advancedResponsePayload("invalid-utf8", &eventcategorypb.Response{
		RetType: new(int32(0)), RetMsg: new(string([]byte{0xff})),
	}); err == nil {
		t.Fatal("invalid UTF-8 advanced response marshaled")
	}
}

func TestComboTradingClientCompleteResponseShapes(t *testing.T) {
	header := testTrdHeader(1001)
	qty := 1.0
	orderType := int32(trdcommonpb.OrderType_OrderType_Normal)
	request := &trdgetcombomaxpb.C2S{
		Header: header, Qty: &qty, OrderType: &orderType,
	}
	if _, err := (*Client)(nil).GetComboMaxTrdQtys(t.Context(), nil); err == nil {
		t.Fatal("nil combo max request succeeded")
	}
	if _, err := (*Client)(nil).PlaceComboOrder(t.Context(), nil); err == nil {
		t.Fatal("nil combo place request succeeded")
	}
	noConn := New(Config{})
	if _, err := noConn.PlaceComboOrder(t.Context(), &trdplacecombopb.C2S{
		Header: header, Qty: &qty, OrderType: &orderType,
	}); err == nil {
		t.Fatal("combo place without connID succeeded")
	}

	t.Run("empty responses", func(t *testing.T) {
		c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
			ProtoTrdGetComboMaxTrdQtys: func(codec.Frame) (proto.Message, error) {
				return &trdgetcombomaxpb.Response{RetType: new(int32(0))}, nil
			},
			ProtoTrdPlaceComboOrder: func(codec.Frame) (proto.Message, error) {
				return &trdplacecombopb.Response{RetType: new(int32(0))}, nil
			},
		})
		maximum, err := c.GetComboMaxTrdQtys(ctx, request)
		if err != nil || maximum == nil {
			t.Fatalf("empty combo max = %#v, %v", maximum, err)
		}
		orderID, err := c.PlaceComboOrder(ctx, &trdplacecombopb.C2S{
			Header: header, Qty: &qty, OrderType: &orderType,
		})
		if err != nil || orderID != "" {
			t.Fatalf("empty combo place = %q, %v", orderID, err)
		}
	})

	t.Run("nested empty and populated", func(t *testing.T) {
		call := 0
		c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
			ProtoTrdGetComboMaxTrdQtys: func(codec.Frame) (proto.Message, error) {
				call++
				s2c := &trdgetcombomaxpb.S2C{Header: header}
				if call > 1 {
					s2c.MaxTrdQtys = &trdcommonpb.ComboMaxTrdQtys{BuyPowerDecrease: new(12.5)}
				}
				return &trdgetcombomaxpb.Response{RetType: new(int32(0)), S2C: s2c}, nil
			},
			ProtoTrdPlaceComboOrder: func(codec.Frame) (proto.Message, error) {
				return &trdplacecombopb.Response{
					RetType: new(int32(0)),
					S2C: &trdplacecombopb.S2C{
						Header: header, OrderIDEx: new("combo-order"),
					},
				}, nil
			},
		})
		if maximum, err := c.GetComboMaxTrdQtys(ctx, request); err != nil || maximum == nil {
			t.Fatalf("nested empty combo max = %#v, %v", maximum, err)
		}
		if maximum, err := c.GetComboMaxTrdQtys(ctx, request); err != nil ||
			maximum.GetBuyPowerDecrease() != 12.5 {
			t.Fatalf("populated combo max = %#v, %v", maximum, err)
		}
		orderID, err := c.PlaceComboOrder(ctx, &trdplacecombopb.C2S{
			Header: header, Qty: &qty, OrderType: &orderType,
		})
		if err != nil || orderID != "combo-order" {
			t.Fatalf("populated combo place = %q, %v", orderID, err)
		}
	})

	t.Run("broker errors", func(t *testing.T) {
		c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
			ProtoTrdGetComboMaxTrdQtys: func(codec.Frame) (proto.Message, error) {
				return &trdgetcombomaxpb.Response{
					RetType: new(int32(1)), ErrCode: new(int32(2)), RetMsg: new("denied"),
				}, nil
			},
			ProtoTrdPlaceComboOrder: func(codec.Frame) (proto.Message, error) {
				return &trdplacecombopb.Response{
					RetType: new(int32(1)), ErrCode: new(int32(3)), RetMsg: new("closed"),
				}, nil
			},
		})
		if _, err := c.GetComboMaxTrdQtys(ctx, request); err == nil {
			t.Fatal("combo max broker error was hidden")
		}
		if _, err := c.PlaceComboOrder(ctx, &trdplacecombopb.C2S{
			Header: header, Qty: &qty, OrderType: &orderType,
		}); err == nil {
			t.Fatal("combo place broker error was hidden")
		}
	})
}
