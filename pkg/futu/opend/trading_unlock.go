package opend

import (
	"context"
	"fmt"

	tradeunlockpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdunlocktrade"
)

// UnlockTrade unlocks the trading session so that orders can be placed.
// pwdMD5 is the MD5 hash of the trading password (lowercase hex).
// When unlock is false the call locks the trading session instead.
// (Trd_UnlockTrade, 2005).
func (c *Client) UnlockTrade(ctx context.Context, unlock bool, pwdMD5 string, securityFirm *int32) error {
	c2s := &tradeunlockpb.C2S{
		Unlock: &unlock,
	}
	if pwdMD5 != "" {
		c2s.PwdMD5 = &pwdMD5
	}
	if securityFirm != nil {
		c2s.SecurityFirm = securityFirm
	}

	request := &tradeunlockpb.Request{C2S: c2s}
	var response tradeunlockpb.Response
	if err := c.Call(ctx, ProtoTrdUnlockTrade, request, &response); err != nil {
		return err
	}
	if response.GetRetType() != 0 {
		return fmt.Errorf("opend Trd_UnlockTrade retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	return nil
}
