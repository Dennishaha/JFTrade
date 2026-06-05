package opend

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	getorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	updateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
)

// OrderBookRequest is a request for order book depth (Qot_GetOrderBook, 3012).
type OrderBookRequest struct {
	Security *qotcommonpb.Security
	Num      int32
}

// OrderBookResult wraps a single order book response.
type OrderBookResult struct {
	Security       *qotcommonpb.Security
	Name           string
	SvrRecvTimeBid string
	SvrRecvTimeAsk string
	AskList        []*qotcommonpb.OrderBook
	BidList        []*qotcommonpb.OrderBook
}

// GetOrderBook fetches the current order book depth (Qot_GetOrderBook, 3012).
// The security must already be subscribed with SubType_OrderBook.
func (c *Client) GetOrderBook(ctx context.Context, req OrderBookRequest) (*OrderBookResult, error) {
	request := &getorderbookpb.Request{C2S: &getorderbookpb.C2S{
		Security: req.Security,
		Num:      proto.Int32(req.Num),
	}}
	frame, err := c.callFrame(ctx, ProtoGetOrderBook, request)
	if err != nil {
		return nil, err
	}
	return parseOrderBookResponse(frame.Body)
}

func parseOrderBookResponse(body []byte) (*OrderBookResult, error) {
	result := &OrderBookResult{}
	retType := int32(-400)
	var errCode int32
	retMsg := ""

	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, fmt.Errorf("opend Qot_GetOrderBook bad response tag: %d", n)
		}
		body = body[n:]

		switch num {
		case 1:
			value, m, err := consumeVarintField(body, typ)
			if err != nil {
				return nil, err
			}
			retType = int32(value)
			body = body[m:]
		case 2:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return nil, err
			}
			retMsg = string(value)
			body = body[m:]
		case 3:
			value, m, err := consumeVarintField(body, typ)
			if err != nil {
				return nil, err
			}
			errCode = int32(value)
			body = body[m:]
		case 4:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return nil, err
			}
			if err := parseOrderBookS2C(value, result); err != nil {
				return nil, err
			}
			body = body[m:]
		default:
			m, err := consumeUnknownField(num, typ, body)
			if err != nil {
				return nil, err
			}
			body = body[m:]
		}
	}

	if retType != 0 {
		return nil, fmt.Errorf("opend Qot_GetOrderBook retType=%d errCode=%d retMsg=%s", retType, errCode, retMsg)
	}
	return result, nil
}

func parseOrderBookS2C(body []byte, result *OrderBookResult) error {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return fmt.Errorf("opend Qot_GetOrderBook bad s2c tag: %d", n)
		}
		body = body[n:]

		switch num {
		case 1:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return err
			}
			security := &qotcommonpb.Security{}
			if err := proto.Unmarshal(value, security); err != nil {
				return fmt.Errorf("opend Qot_GetOrderBook unmarshal security: %w", err)
			}
			result.Security = security
			body = body[m:]
		case 2:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return err
			}
			level, err := parseOrderBookLevel(value)
			if err != nil {
				return err
			}
			result.AskList = append(result.AskList, level)
			body = body[m:]
		case 3:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return err
			}
			level, err := parseOrderBookLevel(value)
			if err != nil {
				return err
			}
			result.BidList = append(result.BidList, level)
			body = body[m:]
		case 4:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return err
			}
			result.SvrRecvTimeBid = string(value)
			body = body[m:]
		case 6:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return err
			}
			result.SvrRecvTimeAsk = string(value)
			body = body[m:]
		case 8:
			value, m, err := consumeBytesField(body, typ)
			if err != nil {
				return err
			}
			result.Name = string(value)
			body = body[m:]
		default:
			m, err := consumeUnknownField(num, typ, body)
			if err != nil {
				return err
			}
			body = body[m:]
		}
	}
	return nil
}

func parseOrderBookLevel(body []byte) (*qotcommonpb.OrderBook, error) {
	level := &qotcommonpb.OrderBook{}
	if err := proto.Unmarshal(body, level); err != nil {
		return nil, fmt.Errorf("opend Qot_GetOrderBook unmarshal level: %w", err)
	}
	return level, nil
}

func consumeVarintField(body []byte, typ protowire.Type) (uint64, int, error) {
	if typ != protowire.VarintType {
		return 0, 0, fmt.Errorf("opend Qot_GetOrderBook expected varint field, got wire type %d", typ)
	}
	value, n := protowire.ConsumeVarint(body)
	if n < 0 {
		return 0, 0, fmt.Errorf("opend Qot_GetOrderBook invalid varint field: %d", n)
	}
	return value, n, nil
}

func consumeBytesField(body []byte, typ protowire.Type) ([]byte, int, error) {
	if typ != protowire.BytesType {
		return nil, 0, fmt.Errorf("opend Qot_GetOrderBook expected bytes field, got wire type %d", typ)
	}
	value, n := protowire.ConsumeBytes(body)
	if n < 0 {
		return nil, 0, fmt.Errorf("opend Qot_GetOrderBook invalid bytes field: %d", n)
	}
	return value, n, nil
}

func consumeUnknownField(num protowire.Number, typ protowire.Type, body []byte) (int, error) {
	n := protowire.ConsumeFieldValue(num, typ, body)
	if n < 0 {
		return 0, fmt.Errorf("opend Qot_GetOrderBook invalid field %d: %d", num, n)
	}
	return n, nil
}

// SubscribeOrderBook registers a typed handler for real-time order book push
// updates (Qot_UpdateOrderBook, 3013).
func (c *Client) SubscribeOrderBook(fn func(*updateorderbookpb.S2C)) {
	if fn == nil {
		return
	}
	c.Subscribe(ProtoQotUpdateOrderBook, func(frame codec.Frame) {
		var response updateorderbookpb.Response
		if err := proto.Unmarshal(frame.Body, &response); err != nil {
			return
		}
		if response.GetRetType() != 0 || response.GetS2C() == nil {
			return
		}
		fn(response.GetS2C())
	})
}
