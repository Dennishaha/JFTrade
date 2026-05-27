package opend

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgethistoryorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderfilllist"
	trdgethistoryorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderlist"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
	trdgetorderfeepb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfee"
	trdgetorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfilllist"
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

// GetHistoryOrderList returns historical orders for the specified trading
// header and filter conditions.
func (c *Client) GetHistoryOrderList(ctx context.Context, header *trdcommonpb.TrdHeader, filter *trdcommonpb.TrdFilterConditions, statuses []int32) ([]*trdcommonpb.Order, error) {
	if filter == nil {
		filter = &trdcommonpb.TrdFilterConditions{}
	}
	request := &trdgethistoryorderlistpb.Request{C2S: &trdgethistoryorderlistpb.C2S{Header: header, FilterConditions: filter, FilterStatusList: statuses}}
	var response trdgethistoryorderlistpb.Response
	if err := c.Call(ctx, ProtoTrdGetHistoryOrderList, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetHistoryOrderList retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdcommonpb.Order{}, nil
	}
	return response.GetS2C().GetOrderList(), nil
}

// GetHistoryOrderFillList returns historical fills for the specified trading
// header and filter conditions.
func (c *Client) GetHistoryOrderFillList(ctx context.Context, header *trdcommonpb.TrdHeader, filter *trdcommonpb.TrdFilterConditions) ([]*trdcommonpb.OrderFill, error) {
	if filter == nil {
		filter = &trdcommonpb.TrdFilterConditions{}
	}
	request := &trdgethistoryorderfilllistpb.Request{C2S: &trdgethistoryorderfilllistpb.C2S{Header: header, FilterConditions: filter}}
	var response trdgethistoryorderfilllistpb.Response
	if err := c.Call(ctx, ProtoTrdGetHistoryOrderFillList, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetHistoryOrderFillList retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdcommonpb.OrderFill{}, nil
	}
	return response.GetS2C().GetOrderFillList(), nil
}

// GetOrderFee returns order-fee snapshots for the specified trading header and
// optional order ID extensions.
func (c *Client) GetOrderFee(ctx context.Context, header *trdcommonpb.TrdHeader, orderIDExList []string) ([]*trdcommonpb.OrderFee, error) {
	request := &trdgetorderfeepb.Request{C2S: &trdgetorderfeepb.C2S{Header: header, OrderIdExList: append([]string(nil), orderIDExList...)}}
	var response trdgetorderfeepb.Response
	if err := c.Call(ctx, ProtoTrdGetOrderFee, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetOrderFee retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdcommonpb.OrderFee{}, nil
	}
	return response.GetS2C().GetOrderFeeList(), nil
}

// GetOrderFillList returns the current-day fills for the specified trading
// header and optional filter conditions.
func (c *Client) GetOrderFillList(ctx context.Context, header *trdcommonpb.TrdHeader, filter *trdcommonpb.TrdFilterConditions) ([]*trdcommonpb.OrderFill, error) {
	request := &trdgetorderfilllistpb.Request{C2S: &trdgetorderfilllistpb.C2S{Header: header, FilterConditions: filter, RefreshCache: proto.Bool(false)}}
	var response trdgetorderfilllistpb.Response
	if err := c.Call(ctx, ProtoTrdGetOrderFillList, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetOrderFillList retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdcommonpb.OrderFill{}, nil
	}
	return response.GetS2C().GetOrderFillList(), nil
}

// GetMarginRatio returns margin-ratio data for the requested securities.
func (c *Client) GetMarginRatio(ctx context.Context, header *trdcommonpb.TrdHeader, securities []*qotcommonpb.Security) ([]*trdgetmarginratiopb.MarginRatioInfo, error) {
	request := &trdgetmarginratiopb.Request{C2S: &trdgetmarginratiopb.C2S{Header: header, SecurityList: securities}}
	var response trdgetmarginratiopb.Response
	if err := c.Call(ctx, ProtoTrdGetMarginRatio, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetMarginRatio retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdgetmarginratiopb.MarginRatioInfo{}, nil
	}
	return response.GetS2C().GetMarginRatioInfoList(), nil
}

// GetFlowSummary returns account cash-flow summaries.
func (c *Client) GetFlowSummary(ctx context.Context, header *trdcommonpb.TrdHeader, clearingDate string, cashFlowDirection *int32) ([]*trdflowsummarypb.FlowSummaryInfo, error) {
	request := &trdflowsummarypb.Request{C2S: &trdflowsummarypb.C2S{Header: header, ClearingDate: proto.String(clearingDate)}}
	if cashFlowDirection != nil {
		request.C2S.CashFlowDirection = proto.Int32(*cashFlowDirection)
	}
	var response trdflowsummarypb.Response
	if err := c.Call(ctx, ProtoTrdFlowSummary, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_FlowSummary retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*trdflowsummarypb.FlowSummaryInfo{}, nil
	}
	return response.GetS2C().GetFlowSummaryInfoList(), nil
}

// GetMaxTrdQtys returns the maximum tradable quantity snapshot for the given
// trade request.
func (c *Client) GetMaxTrdQtys(ctx context.Context, requestC2S *trdgetmaxtrdqtyspb.C2S) (*trdcommonpb.MaxTrdQtys, error) {
	if requestC2S == nil {
		return nil, fmt.Errorf("opend Trd_GetMaxTrdQtys request is required")
	}
	request := &trdgetmaxtrdqtyspb.Request{C2S: requestC2S}
	var response trdgetmaxtrdqtyspb.Response
	if err := c.Call(ctx, ProtoTrdGetMaxTrdQtys, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_GetMaxTrdQtys retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil || response.GetS2C().GetMaxTrdQtys() == nil {
		return &trdcommonpb.MaxTrdQtys{}, nil
	}
	return response.GetS2C().GetMaxTrdQtys(), nil
}
