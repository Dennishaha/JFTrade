package opend

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetusersecuritypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecurity"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
)

func TestGetUserSecurityGroupsEncodesAllAndReturnsCustomAndSystemGroups(t *testing.T) {
	received := make(chan *qotgetusersecuritygrouppb.C2S, 1)
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetUserSecurityGroup: func(frame codec.Frame) (proto.Message, error) {
			var request qotgetusersecuritygrouppb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			received <- request.GetC2S()
			return &qotgetusersecuritygrouppb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetusersecuritygrouppb.S2C{GroupList: []*qotgetusersecuritygrouppb.GroupData{
					{GroupName: new("Long Term"), GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom))},
					{GroupName: new("All"), GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_System))},
				}},
			}, nil
		},
	})

	groups, err := client.GetUserSecurityGroups(ctx, qotgetusersecuritygrouppb.GroupType_GroupType_All)
	if err != nil {
		t.Fatalf("GetUserSecurityGroups() error = %v", err)
	}
	if len(groups) != 2 || groups[0].GetGroupName() != "Long Term" || groups[1].GetGroupName() != "All" {
		t.Fatalf("groups = %#v", groups)
	}
	select {
	case request := <-received:
		if request.GetGroupType() != int32(qotgetusersecuritygrouppb.GroupType_GroupType_All) {
			t.Fatalf("groupType = %d, want all", request.GetGroupType())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for GetUserSecurityGroup request")
	}
}

func TestGetUserSecuritiesEncodesTrimmedGroupName(t *testing.T) {
	received := make(chan *qotgetusersecuritypb.C2S, 1)
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetUserSecurity: func(frame codec.Frame) (proto.Message, error) {
			var request qotgetusersecuritypb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			received <- request.GetC2S()
			return &qotgetusersecuritypb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetusersecuritypb.S2C{StaticInfoList: []*qotcommonpb.SecurityStaticInfo{{
					Basic: &qotcommonpb.SecurityStaticBasic{
						Security: hkSecurity("00700"),
						Id:       new(int64(700)),
						LotSize:  new(int32(100)),
						SecType:  new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
						Name:     new("Tencent"),
						ListTime: new("2004-06-16"),
					},
				}}},
			}, nil
		},
	})

	securities, err := client.GetUserSecurities(ctx, "  Long Term  ")
	if err != nil {
		t.Fatalf("GetUserSecurities() error = %v", err)
	}
	if len(securities) != 1 || securities[0].GetBasic().GetId() != 700 {
		t.Fatalf("securities = %#v", securities)
	}
	select {
	case request := <-received:
		if request.GetGroupName() != "Long Term" {
			t.Fatalf("groupName = %q, want trimmed name", request.GetGroupName())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for GetUserSecurity request")
	}
}

func TestUserSecurityMethodsRejectInvalidInputBeforeEncoding(t *testing.T) {
	client := &Client{}
	if _, err := client.GetUserSecurityGroups(t.Context(), qotgetusersecuritygrouppb.GroupType_GroupType_Unknown); err == nil || !strings.Contains(err.Error(), "invalid groupType") {
		t.Fatalf("invalid group type error = %v", err)
	}
	if _, err := client.GetUserSecurities(t.Context(), "  "); err == nil || !strings.Contains(err.Error(), "groupName is required") {
		t.Fatalf("blank group name error = %v", err)
	}
}

func TestUserSecurityMethodsPropagateBusinessErrorsAndEmptyResults(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetUserSecurityGroup: func(codec.Frame) (proto.Message, error) {
			return &qotgetusersecuritygrouppb.Response{
				RetType: new(int32(-1)), ErrCode: new(int32(429)), RetMsg: new("group query rate limited"),
			}, nil
		},
		ProtoGetUserSecurity: func(codec.Frame) (proto.Message, error) {
			return &qotgetusersecuritypb.Response{RetType: new(int32(0))}, nil
		},
	})

	_, err := client.GetUserSecurityGroups(ctx, qotgetusersecuritygrouppb.GroupType_GroupType_All)
	if err == nil || !strings.Contains(err.Error(), "Qot_GetUserSecurityGroup retType=-1 errCode=429 retMsg=group query rate limited") {
		t.Fatalf("business error = %v", err)
	}
	securities, err := client.GetUserSecurities(ctx, "Empty")
	if err != nil || securities == nil || len(securities) != 0 {
		t.Fatalf("empty securities = (%#v, %v), want non-nil empty", securities, err)
	}
}

func TestUserSecurityProtocolIDs(t *testing.T) {
	if ProtoGetUserSecurity != 3213 {
		t.Fatalf("ProtoGetUserSecurity = %d, want 3213", ProtoGetUserSecurity)
	}
	if ProtoGetUserSecurityGroup != 3222 {
		t.Fatalf("ProtoGetUserSecurityGroup = %d, want 3222", ProtoGetUserSecurityGroup)
	}
}
