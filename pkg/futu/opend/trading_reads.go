package opend

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgetorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderlist"
	trdgetpositionlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetpositionlist"
)

// GetFunds returns the latest cached account-funds snapshot for the specified
// trading header.
func (c *Client) GetFunds(ctx context.Context, header *trdcommonpb.TrdHeader) (*trdcommonpb.Funds, error) {
	request := &trdgetfundspb.Request{C2S: &trdgetfundspb.C2S{Header: header}}
	var response trdgetfundspb.Response
	if err := c.Call(ctx, ProtoTrdGetFunds, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetFunds retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil || response.GetS2C().GetFunds() == nil {
		return &trdcommonpb.Funds{}, nil
	}
	return response.GetS2C().GetFunds(), nil
}

// GetPositionList returns the latest cached position list for the specified
// trading header and optional filter conditions.
func (c *Client) GetPositionList(ctx context.Context, header *trdcommonpb.TrdHeader, filter *trdcommonpb.TrdFilterConditions) ([]*trdcommonpb.Position, error) {
	request := &trdgetpositionlistpb.Request{C2S: &trdgetpositionlistpb.C2S{Header: header, FilterConditions: filter}}
	var response trdgetpositionlistpb.Response
	if err := c.Call(ctx, ProtoTrdGetPositionList, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetPositionList retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdcommonpb.Position{}, nil
	}
	return response.GetS2C().GetPositionList(), nil
}

// GetOrderList returns the latest cached order list for the specified trading
// header and optional filter conditions.
func (c *Client) GetOrderList(ctx context.Context, header *trdcommonpb.TrdHeader, filter *trdcommonpb.TrdFilterConditions) ([]*trdcommonpb.Order, error) {
	request := &trdgetorderlistpb.Request{C2S: &trdgetorderlistpb.C2S{Header: header, FilterConditions: filter, RefreshCache: proto.Bool(false)}}
	var response trdgetorderlistpb.Response
	if err := c.Call(ctx, ProtoTrdGetOrderList, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetOrderList retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdcommonpb.Order{}, nil
	}
	return response.GetS2C().GetOrderList(), nil
}
