package opend

import (
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestParseOrderBookResponseCurrentOpenDWireLayout(t *testing.T) {
	security := &qotcommonpb.Security{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
		Code:   new("NVDA"),
	}
	bid := &qotcommonpb.OrderBook{
		Price:       new(215.86),
		Volume:      new(int64(204)),
		OrederCount: new(int32(0)),
	}
	ask := &qotcommonpb.OrderBook{
		Price:       new(215.82),
		Volume:      new(int64(34)),
		OrederCount: new(int32(0)),
	}

	securityBody, err := proto.Marshal(security)
	if err != nil {
		t.Fatalf("marshal security: %v", err)
	}
	bidBody, err := proto.Marshal(bid)
	if err != nil {
		t.Fatalf("marshal bid: %v", err)
	}
	askBody, err := proto.Marshal(ask)
	if err != nil {
		t.Fatalf("marshal ask: %v", err)
	}

	var s2c []byte
	s2c = protowire.AppendTag(s2c, 1, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, securityBody)
	s2c = protowire.AppendTag(s2c, 2, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, askBody)
	s2c = protowire.AppendTag(s2c, 3, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, bidBody)
	s2c = protowire.AppendTag(s2c, 4, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte("2026-06-01 08:24:35.732"))
	s2c = protowire.AppendTag(s2c, 5, protowire.Fixed64Type)
	s2c = protowire.AppendFixed64(s2c, math.Float64bits(1748766275.732))
	s2c = protowire.AppendTag(s2c, 6, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte("2026-06-01 08:24:35.732"))
	s2c = protowire.AppendTag(s2c, 7, protowire.Fixed64Type)
	s2c = protowire.AppendFixed64(s2c, math.Float64bits(1748766275.732))
	s2c = protowire.AppendTag(s2c, 8, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte("英伟达"))

	var body []byte
	body = protowire.AppendTag(body, 1, protowire.VarintType)
	body = protowire.AppendVarint(body, 0)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendBytes(body, nil)
	body = protowire.AppendTag(body, 3, protowire.VarintType)
	body = protowire.AppendVarint(body, 0)
	body = protowire.AppendTag(body, 4, protowire.BytesType)
	body = protowire.AppendBytes(body, s2c)

	result, err := parseOrderBookResponse(body)
	if err != nil {
		t.Fatalf("parseOrderBookResponse: %v", err)
	}
	if result.Security.GetCode() != "NVDA" {
		t.Fatalf("security code = %q", result.Security.GetCode())
	}
	if result.Name != "英伟达" {
		t.Fatalf("name = %q", result.Name)
	}
	if result.SvrRecvTimeBid != "2026-06-01 08:24:35.732" {
		t.Fatalf("bid time = %q", result.SvrRecvTimeBid)
	}
	if result.SvrRecvTimeAsk != "2026-06-01 08:24:35.732" {
		t.Fatalf("ask time = %q", result.SvrRecvTimeAsk)
	}
	if len(result.BidList) != 1 || result.BidList[0].GetPrice() != 215.86 || result.BidList[0].GetVolume() != 204 {
		t.Fatalf("unexpected bid list: %+v", result.BidList)
	}
	if len(result.AskList) != 1 || result.AskList[0].GetPrice() != 215.82 || result.AskList[0].GetVolume() != 34 {
		t.Fatalf("unexpected ask list: %+v", result.AskList)
	}
}

func TestParseOrderBookResponseError(t *testing.T) {
	var body []byte
	body = protowire.AppendTag(body, 1, protowire.VarintType)
	body = protowire.AppendVarint(body, 1)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendBytes(body, []byte("bad order book"))
	body = protowire.AppendTag(body, 3, protowire.VarintType)
	body = protowire.AppendVarint(body, 321)

	_, err := parseOrderBookResponse(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "opend Qot_GetOrderBook retType=1 errCode=321 retMsg=bad order book" {
		t.Fatalf("unexpected error: %s", got)
	}
}
