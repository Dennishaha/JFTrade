package opend

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	getuserinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/getuserinfo"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestGetQuoteRightsRequestsOnlyEntitlementField(t *testing.T) {
	var requestedFlag int32
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetUserInfo: func(frame codec.Frame) (proto.Message, error) {
			var request getuserinfopb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			requestedFlag = request.GetC2S().GetFlag()
			hkRight := int32(qotcommonpb.QotRight_QotRight_Level2)
			usRight := int32(qotcommonpb.QotRight_QotRight_Level3)
			return &getuserinfopb.Response{
				RetType: new(int32(0)),
				S2C: &getuserinfopb.S2C{
					HkQotRight: &hkRight,
					UsQotRight: &usRight,
				},
			}, nil
		},
	})

	info, err := client.GetQuoteRights(ctx)
	if err != nil {
		t.Fatalf("GetQuoteRights() error = %v", err)
	}
	wantFlag := int32(getuserinfopb.UserInfoField_UserInfoField_QotRight)
	if requestedFlag != wantFlag {
		t.Fatalf("GetQuoteRights() flag = %d, want %d", requestedFlag, wantFlag)
	}
	if info.GetHkQotRight() != int32(qotcommonpb.QotRight_QotRight_Level2) ||
		info.GetUsQotRight() != int32(qotcommonpb.QotRight_QotRight_Level3) {
		t.Fatalf("GetUserInfo() = %#v", info)
	}
}

func TestGetQuoteRightsNormalizesMissingPayloadAndErrors(t *testing.T) {
	t.Run("missing payload", func(t *testing.T) {
		client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
			ProtoGetUserInfo: func(codec.Frame) (proto.Message, error) {
				return &getuserinfopb.Response{RetType: new(int32(0))}, nil
			},
		})
		info, err := client.GetQuoteRights(ctx)
		if err != nil || info == nil {
			t.Fatalf("GetQuoteRights() = (%#v, %v)", info, err)
		}
	})

	t.Run("OpenD error", func(t *testing.T) {
		client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
			ProtoGetUserInfo: func(codec.Frame) (proto.Message, error) {
				return &getuserinfopb.Response{
					RetType: new(int32(-1)),
					ErrCode: new(int32(1001)),
					RetMsg:  new("permission query failed"),
				}, nil
			},
		})
		if _, err := client.GetQuoteRights(ctx); err == nil {
			t.Fatal("GetQuoteRights() error = nil")
		}
	})

	t.Run("closed client", func(t *testing.T) {
		client := &Client{}
		if _, err := client.GetQuoteRights(t.Context()); !errors.Is(err, ErrClosed) {
			t.Fatalf("GetQuoteRights() error = %v, want ErrClosed", err)
		}
	})
}
