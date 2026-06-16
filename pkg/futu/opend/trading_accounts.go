package opend

import (
	"context"
	"fmt"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetacclistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetacclist"
)

// GetAccountList returns the trading accounts currently visible to the OpenD
// session. The response is intentionally narrow and leaves higher-level
// environment/market normalization to the exchange adapter layer.
func (c *Client) GetAccountList(ctx context.Context) ([]*trdcommonpb.TrdAcc, error) {
	request := &trdgetacclistpb.Request{C2S: &trdgetacclistpb.C2S{
		UserID:                new(uint64(0)),
		NeedGeneralSecAccount: new(true),
	}}
	var response trdgetacclistpb.Response
	if err := c.Call(ctx, ProtoTrdGetAccList, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetAccList retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdcommonpb.TrdAcc{}, nil
	}
	return response.GetS2C().GetAccList(), nil
}
