package opend

import (
	"context"
	"fmt"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetcombomaxpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetcombomaxtrdqtys"
	trdplacecombopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplacecomboorder"
)

func (c *Client) GetComboMaxTrdQtys(
	ctx context.Context,
	request *trdgetcombomaxpb.C2S,
) (*trdcommonpb.ComboMaxTrdQtys, error) {
	if request == nil {
		return nil, fmt.Errorf("opend GetComboMaxTrdQtys request is required")
	}
	wrapped := &trdgetcombomaxpb.Request{C2S: request}
	var response trdgetcombomaxpb.Response
	if err := c.Call(ctx, ProtoTrdGetComboMaxTrdQtys, wrapped, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf(
			"opend Trd_GetComboMaxTrdQtys retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg(),
		)
	}
	if response.GetS2C() == nil {
		return &trdcommonpb.ComboMaxTrdQtys{}, nil
	}
	if response.GetS2C().GetMaxTrdQtys() == nil {
		return &trdcommonpb.ComboMaxTrdQtys{}, nil
	}
	return response.GetS2C().GetMaxTrdQtys(), nil
}

func (c *Client) PlaceComboOrder(ctx context.Context, request *trdplacecombopb.C2S) (string, error) {
	if request == nil {
		return "", fmt.Errorf("opend PlaceComboOrder request is required")
	}
	if request.PacketID == nil {
		request.PacketID = c.NextPacketID()
	}
	if request.PacketID == nil {
		return "", fmt.Errorf("opend PlaceComboOrder requires InitConnect connID before trade writes")
	}
	wrapped := &trdplacecombopb.Request{C2S: request}
	var response trdplacecombopb.Response
	if err := c.Call(ctx, ProtoTrdPlaceComboOrder, wrapped, &response); err != nil {
		return "", err
	}
	if response.GetRetType() != 0 {
		return "", fmt.Errorf(
			"opend Trd_PlaceComboOrder retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg(),
		)
	}
	if response.GetS2C() == nil {
		return "", nil
	}
	return response.GetS2C().GetOrderIDEx(), nil
}
