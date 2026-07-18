package futu

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
	qotgetusersecuritypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecurity"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
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
	s.basicQuotesConfigured = true
	s.basicQotError = nil
}

func (s *quoteOpenDServer) setBasicQotError(retType int32, errCode int32, retMsg string) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.basicQotError = &qotgetbasicqotpb.Response{
		RetType: new(retType),
		ErrCode: new(errCode),
		RetMsg:  new(retMsg),
	}
}

func (s *quoteOpenDServer) clearBasicQotError() {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.basicQotError = nil
}

func (s *quoteOpenDServer) setSecuritySnapshots(snapshots []*qotgetsecuritysnapshotpb.Snapshot) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.securitySnapshots = append([]*qotgetsecuritysnapshotpb.Snapshot(nil), snapshots...)
	s.securitySnapshotError = nil
}

func (s *quoteOpenDServer) setSecuritySnapshotError(retType int32, errCode int32, retMsg string) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.securitySnapshotError = &qotgetsecuritysnapshotpb.Response{
		RetType: new(retType),
		ErrCode: new(errCode),
		RetMsg:  new(retMsg),
	}
}

func (s *quoteOpenDServer) setSearchQuotes(quotes []*qotgetsearchquotepb.SearchQuote) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.searchQuotes = append([]*qotgetsearchquotepb.SearchQuote(nil), quotes...)
	s.searchQuoteError = nil
}

func (s *quoteOpenDServer) setSearchQuoteError(retType int32, errCode int32, retMsg string) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.searchQuoteError = &qotgetsearchquotepb.Response{
		RetType: new(retType),
		ErrCode: new(errCode),
		RetMsg:  new(retMsg),
	}
}

func (s *quoteOpenDServer) setWatchlistData(
	groups []*qotgetusersecuritygrouppb.GroupData,
	securities []*qotcommonpb.SecurityStaticInfo,
) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.watchlistGroups = append([]*qotgetusersecuritygrouppb.GroupData(nil), groups...)
	s.watchlistSecurities = append([]*qotcommonpb.SecurityStaticInfo(nil), securities...)
}

func (s *quoteOpenDServer) lastSearchQuoteRequest() (keyword string, maxCount int32) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	return s.lastSearchKeyword, s.lastSearchMaxCount
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
	if s.securitySnapshotError != nil {
		response := jftradeCheckedTypeAssertion[*qotgetsecuritysnapshotpb.Response](proto.Clone(s.securitySnapshotError))
		s.tradeMu.Unlock()
		return response
	}
	snapshots := append([]*qotgetsecuritysnapshotpb.Snapshot(nil), s.securitySnapshots...)
	s.tradeMu.Unlock()
	return &qotgetsecuritysnapshotpb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetsecuritysnapshotpb.S2C{
			SnapshotList: snapshots,
		},
	}
}

func (s *quoteOpenDServer) searchQuoteResponse(body []byte) *qotgetsearchquotepb.Response {
	request := &qotgetsearchquotepb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetsearchquotepb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	if s.searchQuoteError != nil {
		response := jftradeCheckedTypeAssertion[*qotgetsearchquotepb.Response](proto.Clone(s.searchQuoteError))
		s.tradeMu.Unlock()
		return response
	}
	s.lastSearchKeyword = request.GetC2S().GetKeyword()
	s.lastSearchMaxCount = request.GetC2S().GetMaxCount()
	quotes := append([]*qotgetsearchquotepb.SearchQuote(nil), s.searchQuotes...)
	s.tradeMu.Unlock()
	return &qotgetsearchquotepb.Response{
		RetType: new(int32(0)),
		S2C:     &qotgetsearchquotepb.S2C{SearchQuoteList: quotes},
	}
}

func (s *quoteOpenDServer) userSecurityGroupResponse(body []byte) *qotgetusersecuritygrouppb.Response {
	request := &qotgetusersecuritygrouppb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetusersecuritygrouppb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	if s.watchlistGroupError != nil {
		response := jftradeCheckedTypeAssertion[*qotgetusersecuritygrouppb.Response](
			proto.Clone(s.watchlistGroupError),
		)
		s.tradeMu.Unlock()
		return response
	}
	s.lastGroupType = request.GetC2S().GetGroupType()
	groups := append([]*qotgetusersecuritygrouppb.GroupData(nil), s.watchlistGroups...)
	s.tradeMu.Unlock()
	return &qotgetusersecuritygrouppb.Response{
		RetType: new(int32(0)),
		S2C:     &qotgetusersecuritygrouppb.S2C{GroupList: groups},
	}
}

func (s *quoteOpenDServer) userSecurityResponse(body []byte) *qotgetusersecuritypb.Response {
	request := &qotgetusersecuritypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetusersecuritypb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	s.lastGroupName = request.GetC2S().GetGroupName()
	securities := append([]*qotcommonpb.SecurityStaticInfo(nil), s.watchlistSecurities...)
	s.tradeMu.Unlock()
	return &qotgetusersecuritypb.Response{
		RetType: new(int32(0)),
		S2C:     &qotgetusersecuritypb.S2C{StaticInfoList: securities},
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
	configured := s.basicQuotesConfigured
	s.tradeMu.Unlock()
	if !configured {
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
	if configured {
		return quotes
	}
	return basicQotListForSecurities(securities)
}
