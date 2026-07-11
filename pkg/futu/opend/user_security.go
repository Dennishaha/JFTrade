package opend

import (
	"context"
	"fmt"
	"strings"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetusersecuritypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecurity"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
)

// GetUserSecurityGroups returns the OpenD watchlist groups matching groupType
// (Qot_GetUserSecurityGroup, 3222). GroupType_All includes custom and system
// groups and preserves the order returned by OpenD.
func (c *Client) GetUserSecurityGroups(
	ctx context.Context,
	groupType qotgetusersecuritygrouppb.GroupType,
) ([]*qotgetusersecuritygrouppb.GroupData, error) {
	switch groupType {
	case qotgetusersecuritygrouppb.GroupType_GroupType_Custom,
		qotgetusersecuritygrouppb.GroupType_GroupType_System,
		qotgetusersecuritygrouppb.GroupType_GroupType_All:
	default:
		return nil, fmt.Errorf("opend Qot_GetUserSecurityGroup: invalid groupType=%d", groupType)
	}

	value := int32(groupType)
	request := &qotgetusersecuritygrouppb.Request{C2S: &qotgetusersecuritygrouppb.C2S{
		GroupType: &value,
	}}
	var response qotgetusersecuritygrouppb.Response
	if err := c.Call(ctx, ProtoGetUserSecurityGroup, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetUserSecurityGroup retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*qotgetusersecuritygrouppb.GroupData{}, nil
	}
	return response.GetS2C().GetGroupList(), nil
}

// GetUserSecurities returns the static security records in the named OpenD
// watchlist group (Qot_GetUserSecurity, 3213). OpenD resolves duplicate group
// names to the first matching group, so callers must reject ambiguous names
// before invoking this method.
func (c *Client) GetUserSecurities(
	ctx context.Context,
	groupName string,
) ([]*qotcommonpb.SecurityStaticInfo, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil, fmt.Errorf("opend Qot_GetUserSecurity: groupName is required")
	}

	request := &qotgetusersecuritypb.Request{C2S: &qotgetusersecuritypb.C2S{
		GroupName: &groupName,
	}}
	var response qotgetusersecuritypb.Response
	if err := c.Call(ctx, ProtoGetUserSecurity, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetUserSecurity retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*qotcommonpb.SecurityStaticInfo{}, nil
	}
	return response.GetS2C().GetStaticInfoList(), nil
}
