package opend

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	historyklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
)

// KLineRequest is a request for real-time or subscribed K-lines (Qot_GetKL, 3006).
type KLineRequest struct {
	Security  *qotcommonpb.Security
	RehabType qotcommonpb.RehabType
	KLType    qotcommonpb.KLType
	ReqNum    int32
}

// KLineResult wraps a single real-time K-line response.
type KLineResult struct {
	Security *qotcommonpb.Security
	Name     string
	KLines   []*qotcommonpb.KLine
}

// GetKL fetches the latest real-time K-line batch (Qot_GetKL, 3006).
func (c *Client) GetKL(ctx context.Context, req KLineRequest) (*KLineResult, error) {
	request := &qotgetklpb.Request{C2S: &qotgetklpb.C2S{
		RehabType: proto.Int32(int32(req.RehabType)),
		KlType:    proto.Int32(int32(req.KLType)),
		Security:  req.Security,
		ReqNum:    proto.Int32(req.ReqNum),
	}}
	var response qotgetklpb.Response
	if err := c.Call(ctx, ProtoGetKL, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetKL retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return &KLineResult{}, nil
	}
	return &KLineResult{
		Security: response.GetS2C().GetSecurity(),
		Name:     response.GetS2C().GetName(),
		KLines:   response.GetS2C().GetKlList(),
	}, nil
}

// HistoryKLineRequest is a request for historical K-lines (Qot_RequestHistoryKL, 3103).
type HistoryKLineRequest struct {
	Security     *qotcommonpb.Security
	RehabType    qotcommonpb.RehabType
	KLType       qotcommonpb.KLType
	BeginTime    string
	EndTime      string
	MaxAckKLNum  *int32
	NeedKLFields *int64
	NextReqKey   []byte
	ExtendedTime *bool
	Session      *int32
}

// HistoryKLineResult wraps a historical K-line query result.
type HistoryKLineResult struct {
	Security   *qotcommonpb.Security
	Name       string
	KLines     []*qotcommonpb.KLine
	NextReqKey []byte
}

// RequestHistoryKL fetches historical K-lines with pagination via nextReqKey
// (Qot_RequestHistoryKL, 3103).
func (c *Client) RequestHistoryKL(ctx context.Context, req HistoryKLineRequest) (*HistoryKLineResult, error) {
	c2s := &historyklpb.C2S{
		RehabType: proto.Int32(int32(req.RehabType)),
		KlType:    proto.Int32(int32(req.KLType)),
		Security:  req.Security,
		BeginTime: proto.String(req.BeginTime),
		EndTime:   proto.String(req.EndTime),
	}

	if req.MaxAckKLNum != nil {
		c2s.MaxAckKLNum = proto.Int32(*req.MaxAckKLNum)
	}
	if req.NeedKLFields != nil {
		c2s.NeedKLFieldsFlag = proto.Int64(*req.NeedKLFields)
	}
	if len(req.NextReqKey) > 0 {
		c2s.NextReqKey = req.NextReqKey
	}
	if req.ExtendedTime != nil {
		c2s.ExtendedTime = proto.Bool(*req.ExtendedTime)
	}
	if req.Session != nil {
		c2s.Session = proto.Int32(*req.Session)
	}

	request := &historyklpb.Request{C2S: c2s}
	var response historyklpb.Response
	if err := c.Call(ctx, ProtoRequestHistoryKL, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_RequestHistoryKL retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return &HistoryKLineResult{}, nil
	}
	return &HistoryKLineResult{
		Security:   response.GetS2C().GetSecurity(),
		Name:       response.GetS2C().GetName(),
		KLines:     response.GetS2C().GetKlList(),
		NextReqKey: response.GetS2C().GetNextReqKey(),
	}, nil
}
