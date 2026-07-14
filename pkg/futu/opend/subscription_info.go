package opend

import (
	"context"
	"fmt"

	qotgetsubinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsubinfo"
)

// GetSubInfo returns OpenD's subscription state for the current connection or
// all connections. It is a read-only diagnostic call.
func (c *Client) GetSubInfo(ctx context.Context, allConnections bool) (*qotgetsubinfopb.S2C, error) {
	request := &qotgetsubinfopb.Request{C2S: &qotgetsubinfopb.C2S{
		IsReqAllConn: new(allConnections),
	}}
	var response qotgetsubinfopb.Response
	if err := c.Call(ctx, ProtoGetSubInfo, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetSubInfo retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return &qotgetsubinfopb.S2C{}, nil
	}
	return response.GetS2C(), nil
}
