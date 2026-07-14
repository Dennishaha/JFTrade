package opend

import (
	"strings"
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

func TestGetSubInfoSurfacesOpenDErrorsAndNormalizesMissingPayload(t *testing.T) {
	t.Run("OpenD business error includes diagnostics", func(t *testing.T) {
		client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
			ProtoGetSubInfo: func(codec.Frame) (proto.Message, error) {
				return &qotgetsubinfopb.Response{
					RetType: new(int32(-1)),
					ErrCode: new(int32(429)),
					RetMsg:  new("quota query rate limited"),
				}, nil
			},
		})
		if _, err := client.GetSubInfo(ctx, false); err == nil || !strings.Contains(err.Error(), "retType=-1 errCode=429 retMsg=quota query rate limited") {
			t.Fatalf("GetSubInfo business error = %v", err)
		}
	})

	t.Run("missing payload is an empty subscription snapshot", func(t *testing.T) {
		client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
			ProtoGetSubInfo: func(codec.Frame) (proto.Message, error) {
				return &qotgetsubinfopb.Response{RetType: new(int32(0))}, nil
			},
		})
		info, err := client.GetSubInfo(ctx, false)
		if err != nil || info == nil || info.GetTotalUsedQuota() != 0 || info.GetRemainQuota() != 0 {
			t.Fatalf("GetSubInfo missing payload = (%#v, %v)", info, err)
		}
	})
}
