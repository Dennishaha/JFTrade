package opend

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	updateklinepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractkline"
	updateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractorderbook"
	updatetickerpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractticker"
)

func TestPredictionPushSubscribersDispatchOnlySuccessfulTypedUpdates(t *testing.T) {
	client := New(Config{})
	orderBookCalls, klineCalls, tickerCalls := 0, 0, 0
	client.SubscribeEventContractOrderBook(func(value *updateorderbookpb.S2C) {
		if value == nil {
			t.Fatal("nil prediction order-book update")
		}
		orderBookCalls++
	})
	client.SubscribeEventContractKline(func(value *updateklinepb.S2C) {
		if value == nil {
			t.Fatal("nil prediction kline update")
		}
		klineCalls++
	})
	client.SubscribeEventContractTicker(func(value *updatetickerpb.S2C) {
		if value == nil {
			t.Fatal("nil prediction ticker update")
		}
		tickerCalls++
	})

	dispatch := func(protocol uint32, message proto.Message) {
		t.Helper()
		body, err := proto.Marshal(message)
		if err != nil {
			t.Fatalf("marshal prediction push: %v", err)
		}
		client.dispatch(codec.Frame{Header: codec.Header{ProtoID: protocol}, Body: body})
	}
	client.dispatch(codec.Frame{
		Header: codec.Header{ProtoID: ProtoQotUpdateEventContractOrderBook},
		Body:   []byte{0xff},
	})
	dispatch(ProtoQotUpdateEventContractOrderBook, &updateorderbookpb.Response{
		RetType: new(int32(-1)), S2C: &updateorderbookpb.S2C{},
	})
	dispatch(ProtoQotUpdateEventContractOrderBook, &updateorderbookpb.Response{
		RetType: new(int32(0)), S2C: &updateorderbookpb.S2C{},
	})
	dispatch(ProtoQotUpdateEventContractKline, &updateklinepb.Response{
		RetType: new(int32(0)), S2C: &updateklinepb.S2C{},
	})
	dispatch(ProtoQotUpdateEventContractTicker, &updatetickerpb.Response{
		RetType: new(int32(0)), S2C: &updatetickerpb.S2C{},
	})

	if orderBookCalls != 1 || klineCalls != 1 || tickerCalls != 1 {
		t.Fatalf(
			"prediction push callbacks orderBook=%d kline=%d ticker=%d",
			orderBookCalls, klineCalls, tickerCalls,
		)
	}
}

func TestPredictionPushSubscribersIgnoreNilHandlers(t *testing.T) {
	client := New(Config{})
	client.SubscribeEventContractOrderBook(nil)
	client.SubscribeEventContractKline(nil)
	client.SubscribeEventContractTicker(nil)

	// A well-formed push must be dropped rather than dispatched into a nil handler.
	body, err := proto.Marshal(&updateorderbookpb.Response{
		RetType: new(int32(0)), S2C: &updateorderbookpb.S2C{},
	})
	if err != nil {
		t.Fatalf("marshal prediction order-book push: %v", err)
	}
	frame := codec.Frame{
		Header: codec.Header{ProtoID: ProtoQotUpdateEventContractOrderBook},
		Body:   body,
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("dispatch with nil prediction handler panicked: %v", recovered)
			}
		}()
		client.dispatch(frame)
	}()

	// Registering a real handler after the nil registrations must still receive pushes,
	// proving the nil handler did not leave the subscriber slot in a broken state.
	calls := 0
	client.SubscribeEventContractOrderBook(func(value *updateorderbookpb.S2C) {
		if value == nil {
			t.Error("nil prediction order-book update delivered to handler")
		}
		calls++
	})
	client.dispatch(frame)
	if calls != 1 {
		t.Fatalf("order-book handler calls after nil-then-valid subscription = %d, want 1", calls)
	}
}
