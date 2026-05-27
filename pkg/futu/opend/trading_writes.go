package opend

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
	trdsubaccpushpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdsubaccpush"
	trdupdateorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdupdateorder"
	trdupdateorderfillpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdupdateorderfill"
)

type PlaceOrderResult struct {
	OrderID   uint64
	OrderIDEx string
}

type ModifyOrderResult struct {
	OrderID   uint64
	OrderIDEx string
}

func (c *Client) PlaceOrder(ctx context.Context, request *trdplaceorderpb.C2S) (*PlaceOrderResult, error) {
	if request == nil {
		return nil, fmt.Errorf("opend PlaceOrder request is required")
	}
	if request.PacketID == nil {
		request.PacketID = c.NextPacketID()
	}
	if request.PacketID == nil {
		return nil, fmt.Errorf("opend PlaceOrder requires InitConnect connID before trade writes")
	}

	wrapped := &trdplaceorderpb.Request{C2S: request}
	var response trdplaceorderpb.Response
	if err := c.Call(ctx, ProtoTrdPlaceOrder, wrapped, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_PlaceOrder retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return &PlaceOrderResult{}, nil
	}
	return &PlaceOrderResult{
		OrderID:   response.GetS2C().GetOrderID(),
		OrderIDEx: response.GetS2C().GetOrderIDEx(),
	}, nil
}

func (c *Client) ModifyOrder(ctx context.Context, request *trdmodifyorderpb.C2S) (*ModifyOrderResult, error) {
	if request == nil {
		return nil, fmt.Errorf("opend ModifyOrder request is required")
	}
	if request.PacketID == nil {
		request.PacketID = c.NextPacketID()
	}
	if request.PacketID == nil {
		return nil, fmt.Errorf("opend ModifyOrder requires InitConnect connID before trade writes")
	}

	wrapped := &trdmodifyorderpb.Request{C2S: request}
	var response trdmodifyorderpb.Response
	if err := c.Call(ctx, ProtoTrdModifyOrder, wrapped, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Trd_ModifyOrder retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return &ModifyOrderResult{}, nil
	}
	return &ModifyOrderResult{
		OrderID:   response.GetS2C().GetOrderID(),
		OrderIDEx: response.GetS2C().GetOrderIDEx(),
	}, nil
}

func (c *Client) SubscribeAccountPush(ctx context.Context, accountIDs []uint64) error {
	request := &trdsubaccpushpb.Request{C2S: &trdsubaccpushpb.C2S{AccIDList: append([]uint64(nil), accountIDs...)}}
	var response trdsubaccpushpb.Response
	if err := c.Call(ctx, ProtoTrdSubAccPush, request, &response); err != nil {
		return err
	}
	if response.GetRetType() != 0 {
		return fmt.Errorf("opend Trd_SubAccPush retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	return nil
}

func (c *Client) SubscribeOrderUpdate(fn func(*trdcommonpb.TrdHeader, *trdcommonpb.Order)) {
	if fn == nil {
		return
	}
	c.Subscribe(ProtoTrdUpdateOrder, func(frame codec.Frame) {
		var response trdupdateorderpb.Response
		if err := proto.Unmarshal(frame.Body, &response); err != nil || response.GetRetType() != 0 || response.GetS2C() == nil {
			return
		}
		fn(response.GetS2C().GetHeader(), response.GetS2C().GetOrder())
	})
}

func (c *Client) SubscribeOrderFillUpdate(fn func(*trdcommonpb.TrdHeader, *trdcommonpb.OrderFill)) {
	if fn == nil {
		return
	}
	c.Subscribe(ProtoTrdUpdateOrderFill, func(frame codec.Frame) {
		var response trdupdateorderfillpb.Response
		if err := proto.Unmarshal(frame.Body, &response); err != nil || response.GetRetType() != 0 || response.GetS2C() == nil {
			return
		}
		fn(response.GetS2C().GetHeader(), response.GetS2C().GetOrderFill())
	})
}
