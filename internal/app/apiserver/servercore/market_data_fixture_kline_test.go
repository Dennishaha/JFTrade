package servercore

import (
	"time"

	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
)

func (s *marketDataQuoteOpenDServer) setHistoryPages(pages [][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = pages
	s.historyPagesBySession = nil
}

func (s *marketDataQuoteOpenDServer) setHistoryPagesBySession(pages map[int32][][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = nil
	s.historyPagesBySession = make(map[int32][][]*qotcommonpb.KLine, len(pages))
	for session, sessionPages := range pages {
		clonedPages := make([][]*qotcommonpb.KLine, 0, len(sessionPages))
		for _, page := range sessionPages {
			clonedPages = append(clonedPages, append([]*qotcommonpb.KLine(nil), page...))
		}
		s.historyPagesBySession[session] = clonedPages
	}
}

func (s *marketDataQuoteOpenDServer) setCurrentKLines(klines []*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.currentKLines = klines
}

func (s *marketDataQuoteOpenDServer) currentKLCallCount() int {
	return int(s.currentKLCalls.Load())
}

func (s *marketDataQuoteOpenDServer) historyKLResponse(body []byte) *historypb.Response {
	request := &historypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &historypb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}

	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	if len(s.historyPagesBySession) > 0 {
		page := []*qotcommonpb.KLine(nil)
		if pages := s.historyPagesBySession[request.GetC2S().GetSession()]; len(pages) > 0 {
			page = pages[0]
		}
		return &historypb.Response{
			RetType: new(int32(0)),
			S2C: &historypb.S2C{
				Security: request.GetC2S().GetSecurity(),
				KlList:   page,
			},
		}
	}
	page := []*qotcommonpb.KLine(nil)
	if len(s.historyPages) > 0 {
		page = s.historyPages[0]
	}
	return &historypb.Response{
		RetType: new(int32(0)),
		S2C: &historypb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList:   page,
		},
	}
}

func (s *marketDataQuoteOpenDServer) currentKLResponse(body []byte) *qotgetklpb.Response {
	request := &qotgetklpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetklpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}

	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	return &qotgetklpb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetklpb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList:   s.currentKLines,
		},
	}
}

func testMarketDataProtoKLine(at time.Time, open float64, high float64, low float64, close float64, volume int64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       new(at.Format("2006-01-02 15:04:05")),
		Timestamp:  new(float64(at.Unix())),
		IsBlank:    new(false),
		OpenPrice:  new(open),
		HighPrice:  new(high),
		LowPrice:   new(low),
		ClosePrice: new(close),
		Volume:     new(volume),
		Turnover:   new(close * float64(volume)),
	}
}
