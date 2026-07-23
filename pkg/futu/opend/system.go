package opend

import (
	"context"
	"fmt"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	getglobalstatepb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	getuserinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/getuserinfo"
)

// GlobalState holds the OpenD global-state snapshot returned by
// GetGlobalState (1002).
type GlobalState struct {
	MarketHK       int32
	MarketUS       int32
	MarketSH       int32
	MarketSZ       int32
	MarketHKFuture int32
	MarketUSFuture *int32
	MarketSGFuture *int32
	MarketJPFuture *int32
	QotLogined     bool
	TrdLogined     bool
	ServerVer      int32
	ServerBuildNo  int32
	ServerTime     int64
	LocalTime      *float64
	ProgramStatus  *string
	QotSvrIPAddr   *string
	TrdSvrIPAddr   *string
	ConnID         *uint64
}

// GetGlobalState fetches the current OpenD global state, including server
// version, market states, login status, and program status
// (GetGlobalState, 1002).
func (c *Client) GetGlobalState(ctx context.Context) (*GlobalState, error) {
	request := &getglobalstatepb.Request{C2S: &getglobalstatepb.C2S{
		UserID: new(uint64(0)),
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
		gs.MarketUSFuture = new(s2c.GetMarketUSFuture())
	}
	if s2c.MarketSGFuture != nil {
		gs.MarketSGFuture = new(s2c.GetMarketSGFuture())
	}
	if s2c.MarketJPFuture != nil {
		gs.MarketJPFuture = new(s2c.GetMarketJPFuture())
	}
	if s2c.LocalTime != nil {
		gs.LocalTime = new(s2c.GetLocalTime())
	}
	if ps := s2c.GetProgramStatus(); ps != nil {
		gs.ProgramStatus = new(programStatusLabel(ps))
	}
	if s2c.QotSvrIpAddr != nil {
		gs.QotSvrIPAddr = new(s2c.GetQotSvrIpAddr())
	}
	if s2c.TrdSvrIpAddr != nil {
		gs.TrdSvrIPAddr = new(s2c.GetTrdSvrIpAddr())
	}
	if s2c.ConnID != nil {
		gs.ConnID = new(s2c.GetConnID())
	}

	return gs, nil
}

// GetQuoteRights fetches only the current session's market-data entitlements
// from OpenD (GetUserInfo, 1005). Keeping the field mask fixed avoids fetching
// unrelated profile, disclaimer, or web-key data.
func (c *Client) GetQuoteRights(ctx context.Context) (*getuserinfopb.S2C, error) {
	flag := int32(getuserinfopb.UserInfoField_UserInfoField_QotRight)
	request := &getuserinfopb.Request{C2S: &getuserinfopb.C2S{Flag: &flag}}
	var response getuserinfopb.Response
	if err := c.Call(ctx, ProtoGetUserInfo, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf(
			"opend GetUserInfo retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg(),
		)
	}
	if response.GetS2C() == nil {
		return &getuserinfopb.S2C{}, nil
	}
	return response.GetS2C(), nil
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
