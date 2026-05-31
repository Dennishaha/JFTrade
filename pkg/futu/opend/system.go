package opend

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	getglobalstatepb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
)

// GlobalState holds the OpenD global-state snapshot returned by
// GetGlobalState (1002).
type GlobalState struct {
	MarketHK        int32
	MarketUS        int32
	MarketSH        int32
	MarketSZ        int32
	MarketHKFuture  int32
	MarketUSFuture  *int32
	MarketSGFuture  *int32
	MarketJPFuture  *int32
	QotLogined      bool
	TrdLogined      bool
	ServerVer       int32
	ServerBuildNo   int32
	ServerTime      int64
	LocalTime       *float64
	ProgramStatus   *string
	QotSvrIPAddr    *string
	TrdSvrIPAddr    *string
	ConnID          *uint64
}

// GetGlobalState fetches the current OpenD global state, including server
// version, market states, login status, and program status
// (GetGlobalState, 1002).
func (c *Client) GetGlobalState(ctx context.Context) (*GlobalState, error) {
	request := &getglobalstatepb.Request{C2S: &getglobalstatepb.C2S{
		UserID: proto.Uint64(0),
	}}
	var response getglobalstatepb.Response
	if err := c.Call(ctx, ProtoGetGlobalState, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend GetGlobalState retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	s2c := response.GetS2C()
	if s2c == nil {
		return &GlobalState{}, nil
	}

	gs := &GlobalState{
		MarketHK:       s2c.GetMarketHK(),
		MarketUS:       s2c.GetMarketUS(),
		MarketSH:       s2c.GetMarketSH(),
		MarketSZ:       s2c.GetMarketSZ(),
		MarketHKFuture: s2c.GetMarketHKFuture(),
		QotLogined:     s2c.GetQotLogined(),
		TrdLogined:     s2c.GetTrdLogined(),
		ServerVer:      s2c.GetServerVer(),
		ServerBuildNo:  s2c.GetServerBuildNo(),
		ServerTime:     s2c.GetTime(),
	}

	if s2c.MarketUSFuture != nil {
		v := s2c.GetMarketUSFuture()
		gs.MarketUSFuture = &v
	}
	if s2c.MarketSGFuture != nil {
		v := s2c.GetMarketSGFuture()
		gs.MarketSGFuture = &v
	}
	if s2c.MarketJPFuture != nil {
		v := s2c.GetMarketJPFuture()
		gs.MarketJPFuture = &v
	}
	if s2c.LocalTime != nil {
		v := s2c.GetLocalTime()
		gs.LocalTime = &v
	}
	if ps := s2c.GetProgramStatus(); ps != nil {
		v := programStatusLabel(ps)
		gs.ProgramStatus = &v
	}
	if s2c.QotSvrIpAddr != nil {
		v := s2c.GetQotSvrIpAddr()
		gs.QotSvrIPAddr = &v
	}
	if s2c.TrdSvrIpAddr != nil {
		v := s2c.GetTrdSvrIpAddr()
		gs.TrdSvrIPAddr = &v
	}
	if s2c.ConnID != nil {
		v := s2c.GetConnID()
		gs.ConnID = &v
	}

	return gs, nil
}

func programStatusLabel(status *commonpb.ProgramStatus) string {
	if status == nil {
		return "Unavailable"
	}
	value := status.GetType().String()
	if desc := status.GetStrExtDesc(); desc != "" {
		return value + ": " + desc
	}
	return value
}
