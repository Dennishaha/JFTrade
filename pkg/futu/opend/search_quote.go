package opend

import (
	"context"
	"fmt"
	"strings"

	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
)

const maxSearchQuoteCount = 100

// GetSearchQuote searches OpenD's cross-market instrument catalog by code or
// name. The call does not create quote subscriptions.
func (c *Client) GetSearchQuote(ctx context.Context, keyword string, maxCount int32) ([]*qotgetsearchquotepb.SearchQuote, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("opend GetSearchQuote keyword is required")
	}
	if maxCount < 1 || maxCount > maxSearchQuoteCount {
		return nil, fmt.Errorf("opend GetSearchQuote maxCount must be between 1 and %d", maxSearchQuoteCount)
	}

	request := &qotgetsearchquotepb.Request{C2S: &qotgetsearchquotepb.C2S{
		Keyword:  new(keyword),
		MaxCount: new(maxCount),
	}}
	var response qotgetsearchquotepb.Response
	if err := c.Call(ctx, ProtoGetSearchQuote, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend Qot_GetSearchQuote retType=%d errCode=%d retMsg=%s",
			response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	if response.GetS2C() == nil {
		return []*qotgetsearchquotepb.SearchQuote{}, nil
	}
	return response.GetS2C().GetSearchQuoteList(), nil
}
