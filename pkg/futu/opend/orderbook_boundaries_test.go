package opend

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestGetOrderBookRejectsDisconnectedSession(t *testing.T) {
	client := New(Config{})
	_, err := client.GetOrderBook(t.Context(), OrderBookRequest{Security: hkSecurity("00700"), Num: 10})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("GetOrderBook() error = %v, want ErrClosed", err)
	}
}

func TestParseOrderBookResponseRejectsMalformedTopLevelFields(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{"truncated tag", []byte{0x80}, "bad response tag"},
		{"retType wrong wire type", orderBookWireField(1, protowire.BytesType, protowire.AppendBytes(nil, nil)), "expected varint field"},
		{"retType truncated varint", orderBookWireField(1, protowire.VarintType, []byte{0x80}), "invalid varint field"},
		{"retMsg wrong wire type", orderBookWireField(2, protowire.VarintType, protowire.AppendVarint(nil, 0)), "expected bytes field"},
		{"retMsg truncated bytes", orderBookWireField(2, protowire.BytesType, []byte{0x80}), "invalid bytes field"},
		{"errCode wrong wire type", orderBookWireField(3, protowire.BytesType, protowire.AppendBytes(nil, nil)), "expected varint field"},
		{"errCode truncated varint", orderBookWireField(3, protowire.VarintType, []byte{0x80}), "invalid varint field"},
		{"s2c wrong wire type", append(orderBookSuccessPrefix(), orderBookWireField(4, protowire.VarintType, protowire.AppendVarint(nil, 0))...), "expected bytes field"},
		{"s2c malformed tag", orderBookResponseWithS2C([]byte{0x80}), "bad s2c tag"},
		{"unknown field truncated", orderBookWireField(9, protowire.BytesType, []byte{0x80}), "invalid field 9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseOrderBookResponse(tt.body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestParseOrderBookResponseRejectsMalformedS2CFields(t *testing.T) {
	invalidMessage := protowire.AppendBytes(nil, []byte{0x80})
	tests := []struct {
		name string
		s2c  []byte
		want string
	}{
		{"security wrong wire type", orderBookWireField(1, protowire.VarintType, protowire.AppendVarint(nil, 0)), "expected bytes field"},
		{"security truncated bytes", orderBookWireField(1, protowire.BytesType, []byte{0x80}), "invalid bytes field"},
		{"security malformed message", orderBookWireField(1, protowire.BytesType, invalidMessage), "unmarshal security"},
		{"ask wrong wire type", orderBookWireField(2, protowire.VarintType, protowire.AppendVarint(nil, 0)), "expected bytes field"},
		{"ask truncated bytes", orderBookWireField(2, protowire.BytesType, []byte{0x80}), "invalid bytes field"},
		{"ask malformed level", orderBookWireField(2, protowire.BytesType, invalidMessage), "unmarshal level"},
		{"bid wrong wire type", orderBookWireField(3, protowire.VarintType, protowire.AppendVarint(nil, 0)), "expected bytes field"},
		{"bid truncated bytes", orderBookWireField(3, protowire.BytesType, []byte{0x80}), "invalid bytes field"},
		{"bid malformed level", orderBookWireField(3, protowire.BytesType, invalidMessage), "unmarshal level"},
		{"bid receive time wrong wire type", orderBookWireField(4, protowire.VarintType, protowire.AppendVarint(nil, 0)), "expected bytes field"},
		{"bid receive time truncated", orderBookWireField(4, protowire.BytesType, []byte{0x80}), "invalid bytes field"},
		{"ask receive time wrong wire type", orderBookWireField(6, protowire.VarintType, protowire.AppendVarint(nil, 0)), "expected bytes field"},
		{"ask receive time truncated", orderBookWireField(6, protowire.BytesType, []byte{0x80}), "invalid bytes field"},
		{"name wrong wire type", orderBookWireField(8, protowire.VarintType, protowire.AppendVarint(nil, 0)), "expected bytes field"},
		{"name truncated", orderBookWireField(8, protowire.BytesType, []byte{0x80}), "invalid bytes field"},
		{"unknown field truncated", orderBookWireField(9, protowire.BytesType, []byte{0x80}), "invalid field 9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseOrderBookResponse(orderBookResponseWithS2C(tt.s2c))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestParseOrderBookResponseSkipsValidUnknownFields(t *testing.T) {
	var s2c []byte
	s2c = append(s2c, orderBookWireField(9, protowire.VarintType, protowire.AppendVarint(nil, 7))...)
	s2c = append(s2c, orderBookWireField(8, protowire.BytesType, protowire.AppendBytes(nil, []byte("Tencent")))...)
	body := orderBookResponseWithS2C(s2c)
	body = append(body, orderBookWireField(10, protowire.Fixed32Type, protowire.AppendFixed32(nil, 9))...)

	result, err := parseOrderBookResponse(body)
	if err != nil {
		t.Fatalf("parseOrderBookResponse() error = %v", err)
	}
	if result.Name != "Tencent" || result.Security != nil || len(result.AskList) != 0 || len(result.BidList) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseOrderBookLevelAcceptsValidRequiredFields(t *testing.T) {
	level := &qotcommonpb.OrderBook{Price: new(320.5), Volume: new(int64(200)), OrederCount: new(int32(3))}
	body, err := proto.Marshal(level)
	if err != nil {
		t.Fatalf("marshal level: %v", err)
	}
	parsed, err := parseOrderBookLevel(body)
	if err != nil {
		t.Fatalf("parseOrderBookLevel() error = %v", err)
	}
	if parsed.GetPrice() != 320.5 || parsed.GetVolume() != 200 || parsed.GetOrederCount() != 3 {
		t.Fatalf("parsed level = %#v", parsed)
	}
}

func orderBookWireField(number protowire.Number, wireType protowire.Type, value []byte) []byte {
	field := protowire.AppendTag(nil, number, wireType)
	return append(field, value...)
}

func orderBookSuccessPrefix() []byte {
	return orderBookWireField(1, protowire.VarintType, protowire.AppendVarint(nil, 0))
}

func orderBookResponseWithS2C(s2c []byte) []byte {
	body := orderBookSuccessPrefix()
	return append(body, orderBookWireField(4, protowire.BytesType, protowire.AppendBytes(nil, s2c))...)
}
