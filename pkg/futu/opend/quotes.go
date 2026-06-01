package opend

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
)

// QuoteSubRequest aggregates the parameters for a QotSub (3001) call.
type QuoteSubRequest struct {
	Securities           []*qotcommonpb.Security
	SubTypes             []qotcommonpb.SubType
	IsSubscribe          bool
	IsRegPush            *bool
	RegPushRehabTypes    []qotcommonpb.RehabType
	IsFirstPush          *bool
	IsUnsubAll           bool
	ExtendedTime         *bool
	Session              *int32
	IsSubOrderBookDetail *bool // 仅用于港股 SF 行情权限下订阅港股 ORDER_BOOK 类型
}

// Subscribe sends a quote subscription request (QotSub, 3001).
// When isSubscribe is false it performs an unsubscription instead.
func (c *Client) SubscribeQuotes(ctx context.Context, req QuoteSubRequest) error {
	subTypeInts := make([]int32, len(req.SubTypes))
	for i, st := range req.SubTypes {
		subTypeInts[i] = int32(st)
	}

	rehabTypeInts := make([]int32, len(req.RegPushRehabTypes))
	for i, rt := range req.RegPushRehabTypes {
		rehabTypeInts[i] = int32(rt)
	}

	c2s := &qotsubpb.C2S{
		SecurityList: req.Securities,
		SubTypeList:  subTypeInts,
		IsSubOrUnSub: proto.Bool(req.IsSubscribe),
		IsUnsubAll:   proto.Bool(req.IsUnsubAll),
	}

	if req.IsRegPush != nil {
		c2s.IsRegOrUnRegPush = proto.Bool(*req.IsRegPush)
	}
	if len(req.RegPushRehabTypes) > 0 {
		c2s.RegPushRehabTypeList = rehabTypeInts
	}
	if req.IsFirstPush != nil {
		c2s.IsFirstPush = proto.Bool(*req.IsFirstPush)
	}
	if req.ExtendedTime != nil {
		c2s.ExtendedTime = proto.Bool(*req.ExtendedTime)
	}
	if req.Session != nil {
		c2s.Session = proto.Int32(*req.Session)
	}
	if req.IsSubOrderBookDetail != nil {
		c2s.IsSubOrderBookDetail = proto.Bool(*req.IsSubOrderBookDetail)
	}

	request := &qotsubpb.Request{C2S: c2s}
	var response qotsubpb.Response
	if err := c.Call(ctx, ProtoQotSub, request, &response); err != nil {
		return err
	}
	if response.GetRetType() != 0 {
		return fmt.Errorf("opend QotSub retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	return nil
}

// GetBasicQot pulls the latest basic quote snapshots for the given securities
// (Qot_GetBasicQot, 3004).
func (c *Client) GetBasicQot(ctx context.Context, securities []*qotcommonpb.Security) ([]*qotcommonpb.BasicQot, error) {
	request := &qotgetbasicqotpb.Request{C2S: &qotgetbasicqotpb.C2S{
		SecurityList: securities,
	}}
	var response qotgetbasicqotpb.Response
	if err := c.Call(ctx, ProtoGetBasicQot, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetBasicQot retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*qotcommonpb.BasicQot{}, nil
	}
	return response.GetS2C().GetBasicQotList(), nil
}

// SubscribeBasicQot registers a typed handler for real-time basic-quote push
// updates (Qot_UpdateBasicQot, 3005).
func (c *Client) SubscribeBasicQot(fn func([]*qotcommonpb.BasicQot)) {
	if fn == nil {
		return
	}
	c.Subscribe(ProtoQotUpdateBasicQot, func(frame codec.Frame) {
		var response qotupdatebasicqotpb.Response
		if err := proto.Unmarshal(frame.Body, &response); err != nil {
			return
		}
		if response.GetRetType() != 0 || response.GetS2C() == nil {
			return
		}
		fn(response.GetS2C().GetBasicQotList())
	})
}
