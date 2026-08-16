package productfeatures

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
)

func handleCalendarQuery(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		context := typedReadContext(c)
		result, err := svc.QueryCalendar(c.Request.Context(), service.CalendarRequest{
			ReadContext: context, Operation: c.Query("operation"), Date: c.Query("date"),
			BeginDate: c.Query("beginDate"), EndDate: c.Query("endDate"), Sort: c.Query("sort"),
			StockScope: c.Query("stockScope"), MarketCapMin: c.Query("marketCapMin"),
			MarketCapMax: c.Query("marketCapMax"), OptionVolumeMin: c.Query("optionVolumeMin"),
			OptionVolumeMax: c.Query("optionVolumeMax"), IVMin: c.Query("ivMin"), IVMax: c.Query("ivMax"),
			IVRankMin: c.Query("ivRankMin"), IVRankMax: c.Query("ivRankMax"),
			IVPercentileMin: c.Query("ivPercentileMin"), IVPercentileMax: c.Query("ivPercentileMax"),
			Refresh: c.Query("refresh") == "true",
		})
		writeDocumentResult(c, result, err)
	}
}

func handleRankingsQuery(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := svc.QueryRankings(c.Request.Context(), service.RankingsRequest{
			ReadContext: typedReadContext(c), Operation: c.Query("operation"),
			Direction: c.Query("direction"), PlateType: c.Query("plateType"),
			Refresh: c.Query("refresh") == "true",
		})
		writeDocumentResult(c, result, err)
	}
}

func handleInstrumentResearchQuery(
	svc *service.Service,
	family service.InstrumentResearchFamily,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := svc.QueryInstrumentResearch(c.Request.Context(), service.InstrumentResearchRequest{
			ReadContext: typedReadContext(c), Family: family,
			InstrumentID: strings.ToUpper(strings.TrimSpace(c.Param("instrumentId"))),
			Operation:    c.Query("operation"), Statement: c.Query("statement"),
			Refresh: c.Query("refresh") == "true",
		})
		writeDocumentResult(c, result, err)
	}
}

func typedReadContext(c *gin.Context) service.ReadContext {
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	return service.ReadContext{
		BrokerID: c.Query("brokerId"), AccountID: c.Query("accountId"),
		TradingEnvironment: c.Query("tradingEnvironment"),
		Market:             strings.ToUpper(strings.TrimSpace(c.Query("market"))),
		Cursor:             c.Query("cursor"), PageSize: pageSize,
	}
}

func writeDocumentResult(c *gin.Context, result *service.DocumentResult, err error) {
	if err != nil {
		writeQueryError(c, err)
		return
	}
	projected, err := result.FeatureResult()
	if err != nil {
		httpserver.WriteError(c, http.StatusBadGateway, "BROKER_FEATURE_FAILED", err.Error())
		return
	}
	httpserver.WriteOK(c, projected)
}
