package opend

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
)

func TestGetSearchQuoteSendsKeywordAndReturnsCandidates(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetSearchQuote: func(frame codec.Frame) (proto.Message, error) {
			var request qotgetsearchquotepb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetKeyword() != "苹果" || request.GetC2S().GetMaxCount() != 100 {
				t.Fatalf("GetSearchQuote request = %#v", request.GetC2S())
			}
			return &qotgetsearchquotepb.Response{
				RetType: new(int32(0)),
				S2C: &qotgetsearchquotepb.S2C{SearchQuoteList: []*qotgetsearchquotepb.SearchQuote{
					{
						Market:    new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
						Code:      new("AAPL"),
						Name:      new("苹果"),
						SecType:   new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
						IsWatched: new(true),
					},
				}},
			}, nil
		},
	})

	results, err := client.GetSearchQuote(ctx, "  苹果  ", 100)
	if err != nil {
		t.Fatalf("GetSearchQuote: %v", err)
	}
	if len(results) != 1 || results[0].GetCode() != "AAPL" || results[0].GetName() != "苹果" || !results[0].GetIsWatched() {
		t.Fatalf("GetSearchQuote results = %#v", results)
	}
}

func TestGetSearchQuoteValidatesInputAndPreservesOpenDErrors(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetSearchQuote: func(codec.Frame) (proto.Message, error) {
			return &qotgetsearchquotepb.Response{
				RetType: new(int32(-1)),
				ErrCode: new(int32(400)),
				RetMsg:  new("too frequent"),
			}, nil
		},
	})

	for _, test := range []struct {
		keyword string
		limit   int32
		want    string
	}{
		{" ", 10, "keyword is required"},
		{"AAPL", 0, "maxCount must be between"},
		{"AAPL", 101, "maxCount must be between"},
	} {
		if _, err := client.GetSearchQuote(ctx, test.keyword, test.limit); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("GetSearchQuote(%q, %d) error = %v, want %q", test.keyword, test.limit, err, test.want)
		}
	}
	if _, err := client.GetSearchQuote(ctx, "AAPL", 10); err == nil || !strings.Contains(err.Error(), "too frequent") {
		t.Fatalf("GetSearchQuote OpenD error = %v", err)
	}
}
