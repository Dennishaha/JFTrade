package futu

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
	tradeunlockpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdunlocktrade"
)

func (s *quoteOpenDServer) setStaticInfos(infos []*qotcommonpb.SecurityStaticInfo) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.staticInfos = append([]*qotcommonpb.SecurityStaticInfo(nil), infos...)
	s.staticInfoError = nil
}

func (s *quoteOpenDServer) setStaticInfoError(retType int32, errCode int32, retMsg string) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.staticInfoError = &qotgetstaticinfopb.Response{
		RetType: new(retType),
		ErrCode: new(errCode),
		RetMsg:  new(retMsg),
	}
}

func (s *quoteOpenDServer) setBasicQuotes(quotes []*qotcommonpb.BasicQot) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.basicQuotes = append([]*qotcommonpb.BasicQot(nil), quotes...)
}

func (s *quoteOpenDServer) setSecuritySnapshots(snapshots []*qotgetsecuritysnapshotpb.Snapshot) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.securitySnapshots = append([]*qotgetsecuritysnapshotpb.Snapshot(nil), snapshots...)
}

func (s *quoteOpenDServer) setOrderBookSnapshot(snapshot *qotgetorderbookpb.S2C) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	if snapshot == nil {
		s.orderBookSnapshot = nil
		return
	}
	s.orderBookSnapshot = jftradeCheckedTypeAssertion[*qotgetorderbookpb.S2C](proto.Clone(snapshot))
}

func (s *quoteOpenDServer) staticInfoResponse(body []byte) *qotgetstaticinfopb.Response {
	request := &qotgetstaticinfopb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetstaticinfopb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	if s.staticInfoError != nil {
		response := jftradeCheckedTypeAssertion[*qotgetstaticinfopb.Response](proto.Clone(s.staticInfoError))
		s.tradeMu.Unlock()
		return response
	}
	infos := append([]*qotcommonpb.SecurityStaticInfo(nil), s.staticInfos...)
	s.tradeMu.Unlock()
	return &qotgetstaticinfopb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetstaticinfopb.S2C{
			StaticInfoList: infos,
		},
	}
}

func (s *quoteOpenDServer) securitySnapshotResponse(body []byte) *qotgetsecuritysnapshotpb.Response {
	request := &qotgetsecuritysnapshotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetsecuritysnapshotpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	snapshots := append([]*qotgetsecuritysnapshotpb.Snapshot(nil), s.securitySnapshots...)
	s.tradeMu.Unlock()
	return &qotgetsecuritysnapshotpb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetsecuritysnapshotpb.S2C{
			SnapshotList: snapshots,
		},
	}
}

func (s *quoteOpenDServer) orderBookResponseBody(body []byte) ([]byte, error) {
	request := &qotgetorderbookpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return nil, err
	}
	s.tradeMu.Lock()
	s.lastOrderBook = request.GetC2S()
	snapshot := s.orderBookSnapshot
	s.tradeMu.Unlock()
	if snapshot == nil {
		snapshot = &qotgetorderbookpb.S2C{Security: request.GetC2S().GetSecurity()}
	}

	securityBody, err := proto.Marshal(snapshot.GetSecurity())
	if err != nil {
		return nil, fmt.Errorf("marshal order-book security: %w", err)
	}

	var s2c []byte
	s2c = protowire.AppendTag(s2c, 1, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, securityBody)
	for _, level := range snapshot.GetOrderBookAskList() {
		levelBody, err := proto.Marshal(level)
		if err != nil {
			return nil, fmt.Errorf("marshal ask level: %w", err)
		}
		s2c = protowire.AppendTag(s2c, 2, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, levelBody)
	}
	for _, level := range snapshot.GetOrderBookBidList() {
		levelBody, err := proto.Marshal(level)
		if err != nil {
			return nil, fmt.Errorf("marshal bid level: %w", err)
		}
		s2c = protowire.AppendTag(s2c, 3, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, levelBody)
	}
	if value := snapshot.GetSvrRecvTimeBid(); value != "" {
		s2c = protowire.AppendTag(s2c, 4, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, []byte(value))
	}
	if value := snapshot.GetSvrRecvTimeAsk(); value != "" {
		s2c = protowire.AppendTag(s2c, 6, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, []byte(value))
	}
	if value := snapshot.GetName(); value != "" {
		s2c = protowire.AppendTag(s2c, 8, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, []byte(value))
	}

	var response []byte
	response = protowire.AppendTag(response, 1, protowire.VarintType)
	response = protowire.AppendVarint(response, 0)
	response = protowire.AppendTag(response, 2, protowire.BytesType)
	response = protowire.AppendBytes(response, nil)
	response = protowire.AppendTag(response, 3, protowire.VarintType)
	response = protowire.AppendVarint(response, 0)
	response = protowire.AppendTag(response, 4, protowire.BytesType)
	response = protowire.AppendBytes(response, s2c)
	return response, nil
}

func (s *quoteOpenDServer) unlockTradeResponse(body []byte) *tradeunlockpb.Response {
	request := &tradeunlockpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &tradeunlockpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	s.lastUnlockTrade = request.GetC2S()
	s.tradeMu.Unlock()
	return &tradeunlockpb.Response{RetType: new(int32(0))}
}

func (s *quoteOpenDServer) basicQotResponseForSecurities(securities []*qotcommonpb.Security) []*qotcommonpb.BasicQot {
	s.tradeMu.Lock()
	overrides := append([]*qotcommonpb.BasicQot(nil), s.basicQuotes...)
	s.tradeMu.Unlock()
	if len(overrides) == 0 {
		return basicQotListForSecurities(securities)
	}

	byCanonical := make(map[string]*qotcommonpb.BasicQot, len(overrides))
	for _, quote := range overrides {
		if quote == nil {
			continue
		}
		canonical, err := futuSymbolFromSecurity(quote.GetSecurity())
		if err != nil {
			continue
		}
		byCanonical[canonical] = quote
	}

	quotes := make([]*qotcommonpb.BasicQot, 0, len(securities))
	for _, security := range securities {
		canonical, err := futuSymbolFromSecurity(security)
		if err != nil {
			continue
		}
		if quote := byCanonical[canonical]; quote != nil {
			quotes = append(quotes, quote)
		}
	}
	if len(quotes) > 0 {
		return quotes
	}
	return basicQotListForSecurities(securities)
}
