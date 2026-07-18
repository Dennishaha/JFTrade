package opend

import (
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	updateklinepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractkline"
	updateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractorderbook"
	updatetickerpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractticker"
)

func (c *Client) SubscribeEventContractOrderBook(
	fn func(*updateorderbookpb.S2C),
) {
	subscribePredictionPush(c, ProtoQotUpdateEventContractOrderBook, func(frame codec.Frame) {
		var response updateorderbookpb.Response
		if proto.Unmarshal(frame.Body, &response) == nil &&
			response.GetRetType() == 0 && response.GetS2C() != nil {
			fn(response.GetS2C())
		}
	}, fn != nil)
}

func (c *Client) SubscribeEventContractKline(fn func(*updateklinepb.S2C)) {
	subscribePredictionPush(c, ProtoQotUpdateEventContractKline, func(frame codec.Frame) {
		var response updateklinepb.Response
		if proto.Unmarshal(frame.Body, &response) == nil &&
			response.GetRetType() == 0 && response.GetS2C() != nil {
			fn(response.GetS2C())
		}
	}, fn != nil)
}

func (c *Client) SubscribeEventContractTicker(fn func(*updatetickerpb.S2C)) {
	subscribePredictionPush(c, ProtoQotUpdateEventContractTicker, func(frame codec.Frame) {
		var response updatetickerpb.Response
		if proto.Unmarshal(frame.Body, &response) == nil &&
			response.GetRetType() == 0 && response.GetS2C() != nil {
			fn(response.GetS2C())
		}
	}, fn != nil)
}

func subscribePredictionPush(
	client *Client,
	protocol uint32,
	handler func(codec.Frame),
	enabled bool,
) {
	if client == nil || !enabled {
		return
	}
	client.Subscribe(protocol, handler)
}
