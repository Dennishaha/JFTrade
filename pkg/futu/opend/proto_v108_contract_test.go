package opend

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotgetorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
)

func TestProto108OrderBookFieldNumbers(t *testing.T) {
	assertProtoFieldNumber(t, (&qotgetorderbookpb.C2S{}).ProtoReflect().Descriptor(), "orderBookType", 4)
	assertProtoFieldNumber(t, (&qotgetorderbookpb.S2C{}).ProtoReflect().Descriptor(), "orderBookAskList", 2)
	assertProtoFieldNumber(t, (&qotgetorderbookpb.S2C{}).ProtoReflect().Descriptor(), "orderBookBidList", 3)
	assertProtoFieldNumber(t, (&qotgetorderbookpb.S2C{}).ProtoReflect().Descriptor(), "svrRecvTimeBid", 4)
	assertProtoFieldNumber(t, (&qotgetorderbookpb.S2C{}).ProtoReflect().Descriptor(), "svrRecvTimeAsk", 6)
	assertProtoFieldNumber(t, (&qotgetorderbookpb.S2C{}).ProtoReflect().Descriptor(), "name", 8)
	assertProtoFieldNumber(t, (&qotgetorderbookpb.S2C{}).ProtoReflect().Descriptor(), "orderBookType", 9)
	assertProtoFieldNumber(t, (&qotupdateorderbookpb.S2C{}).ProtoReflect().Descriptor(), "orderBookAskList", 2)
	assertProtoFieldNumber(t, (&qotupdateorderbookpb.S2C{}).ProtoReflect().Descriptor(), "orderBookBidList", 3)
	assertProtoFieldNumber(t, (&qotupdateorderbookpb.S2C{}).ProtoReflect().Descriptor(), "svrRecvTimeBid", 4)
	assertProtoFieldNumber(t, (&qotupdateorderbookpb.S2C{}).ProtoReflect().Descriptor(), "svrRecvTimeAsk", 6)
	assertProtoFieldNumber(t, (&qotupdateorderbookpb.S2C{}).ProtoReflect().Descriptor(), "name", 8)
	assertProtoFieldNumber(t, (&qotupdateorderbookpb.S2C{}).ProtoReflect().Descriptor(), "orderBookType", 9)
}

func TestProto108NotifyFieldNumbers(t *testing.T) {
	assertProtoFieldNumber(t, (&notifypb.GtwEvent{}).ProtoReflect().Descriptor(), "eventType", 1)
	assertProtoFieldNumber(t, (&notifypb.ProgramStatus{}).ProtoReflect().Descriptor(), "programStatus", 1)
	assertProtoFieldNumber(t, (&notifypb.QotRight{}).ProtoReflect().Descriptor(), "hkQotRight", 4)
	assertProtoFieldNumber(t, (&notifypb.QotRight{}).ProtoReflect().Descriptor(), "usQotRight", 5)
	assertProtoFieldNumber(t, (&notifypb.QotRight{}).ProtoReflect().Descriptor(), "shQotRight", 21)
	assertProtoFieldNumber(t, (&notifypb.QotRight{}).ProtoReflect().Descriptor(), "szQotRight", 22)
	assertProtoFieldNumber(t, (&notifypb.QotRight{}).ProtoReflect().Descriptor(), "sgStockQotRight", 24)
	assertProtoFieldNumber(t, (&notifypb.QotRight{}).ProtoReflect().Descriptor(), "myStockQotRight", 25)
	assertProtoFieldNumber(t, (&notifypb.QotRight{}).ProtoReflect().Descriptor(), "jpStockQotRight", 26)
	assertProtoFieldNumber(t, (&notifypb.APIQuota{}).ProtoReflect().Descriptor(), "subQuota", 1)
	assertProtoFieldNumber(t, (&notifypb.APIQuota{}).ProtoReflect().Descriptor(), "historyKLQuota", 2)
	assertProtoFieldNumber(t, (&notifypb.UsedQuota{}).ProtoReflect().Descriptor(), "usedSubQuota", 1)
	assertProtoFieldNumber(t, (&notifypb.UsedQuota{}).ProtoReflect().Descriptor(), "usedKLineQuota", 2)
	assertProtoFieldNumber(t, (&notifypb.S2C{}).ProtoReflect().Descriptor(), "apiQuota", 7)
	assertProtoFieldNumber(t, (&notifypb.S2C{}).ProtoReflect().Descriptor(), "usedQuota", 8)
}

func assertProtoFieldNumber(t *testing.T, descriptor protoreflect.MessageDescriptor, name string, want protoreflect.FieldNumber) {
	t.Helper()
	field := descriptor.Fields().ByName(protoreflect.Name(name))
	if field == nil {
		t.Fatalf("field %s not found", name)
	}
	if field.Number() != want {
		t.Fatalf("field %s number = %d, want %d", name, field.Number(), want)
	}
}
