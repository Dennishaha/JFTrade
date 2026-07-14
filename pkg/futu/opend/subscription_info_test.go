package opend

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	qotgetsubinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsubinfo"
)

func TestGetSubInfoUsesReadOnlySubscriptionProtocol(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetSubInfo: func(frame codec.Frame) (proto.Message, error) {
			var request qotgetsubinfopb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if !request.GetC2S().GetIsReqAllConn() {
				t.Fatal("GetSubInfo should preserve allConnections")
			}
			return &qotgetsubinfopb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetsubinfopb.S2C{
					TotalUsedQuota: new(int32(3)),
					RemainQuota:    new(int32(97)),
				},
			}, nil
		},
	})

	info, err := client.GetSubInfo(ctx, true)
	if err != nil {
		t.Fatalf("GetSubInfo: %v", err)
	}
	if info.GetTotalUsedQuota() != 3 || info.GetRemainQuota() != 97 {
		t.Fatalf("GetSubInfo = %+v", info)
	}
}
