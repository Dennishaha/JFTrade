package opend

import (
	"context"
	"fmt"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
)

// GetStaticInfo retrieves security static-info (name, trading hours, sector,
// warrant/option/future extension data) for the supplied securities
// (Qot_GetStaticInfo, 3202).
func (c *Client) GetStaticInfo(ctx context.Context, securities []*qotcommonpb.Security) ([]*qotcommonpb.SecurityStaticInfo, error) {
	request := &qotgetstaticinfopb.Request{C2S: &qotgetstaticinfopb.C2S{
		SecurityList: securities,
	}}
	var response qotgetstaticinfopb.Response
	if err := c.Call(ctx, ProtoGetStaticInfo, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetStaticInfo retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*qotcommonpb.SecurityStaticInfo{}, nil
	}
	return response.GetS2C().GetStaticInfoList(), nil
}

// GetSecuritySnapshot retrieves snapshots for the supplied securities.
// Each snapshot carries basic BBO + extended data appropriate to the security
// type (equity, warrant, option, index, plate, future, trust)
// (Qot_GetSecuritySnapshot, 3203).
func (c *Client) GetSecuritySnapshot(ctx context.Context, securities []*qotcommonpb.Security) ([]*qotgetsecuritysnapshotpb.Snapshot, error) {
	request := &qotgetsecuritysnapshotpb.Request{C2S: &qotgetsecuritysnapshotpb.C2S{
		SecurityList: securities,
	}}
	var response qotgetsecuritysnapshotpb.Response
	if err := c.Call(ctx, ProtoGetSecuritySnapshot, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetSecuritySnapshot retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*qotgetsecuritysnapshotpb.Snapshot{}, nil
	}
	return response.GetS2C().GetSnapshotList(), nil
}
