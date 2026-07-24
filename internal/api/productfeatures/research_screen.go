package productfeatures

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

// handleResearchScreenCatalog godoc
// @Summary 读取股票筛选因子目录
// @Tags research
// @Produce json
// @Param brokerId query string false "券商 ID"
// @Param market query string false "市场"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Router /api/v1/research/screens/catalog [get]
func handleResearchScreenCatalog() gin.HandlerFunc {
	return func(c *gin.Context) {
		brokerID := strings.ToLower(strings.TrimSpace(c.Query("brokerId")))
		if brokerID != "" && brokerID != "futu" {
			httpserver.WriteError(
				c, http.StatusConflict, "BROKER_CAPABILITY_UNAVAILABLE",
				"the complete stock-screen factor catalog is currently available only for futu",
			)
			return
		}
		market := strings.ToUpper(strings.TrimSpace(c.Query("market")))
		if market != "" {
			if market != "HK" && market != "US" && market != "SH" && market != "SZ" {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "unsupported stock-screen market")
				return
			}
		}
		httpserver.WriteOK(c, researchscreen.CatalogForMarket(market))
	}
}

// handleResearchScreenQuery godoc
// @Summary 执行类型化股票筛选
// @Tags research
// @Accept json
// @Produce json
// @Param request body broker.ScreenQueryV2 true "股票筛选请求"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 429 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/research/screens [post]
func handleResearchScreenQuery(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body broker.ScreenQueryV2
		if err := httpserver.BindStrictJSON(c, &body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid stock-screen request")
			return
		}
		if err := normalizeResearchScreenQuery(&body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		query := broker.FeatureQuery{
			BrokerID: body.BrokerID, AccountID: body.AccountID,
			TradingEnvironment: body.TradingEnvironment,
			Market:             body.Market,
			FeatureID:          broker.FeatureResearchScreen,
			Cursor:             strconv.Itoa(body.Page.Offset),
			PageSize:           body.Page.Limit,
			Params: map[string]any{
				"operation":                "stock_v2",
				"researchScreenDefinition": body.ScreenDefinitionV2,
				"pageFrom":                 body.Page.Offset,
			},
		}
		result, err := svc.Query(c.Request.Context(), query)
		if err != nil {
			writeResearchScreenError(c, err)
			return
		}
		typed, err := typedResearchScreenResult(result)
		if err != nil {
			httpserver.WriteError(c, http.StatusBadGateway, "BROKER_FEATURE_FAILED", err.Error())
			return
		}
		typed.CatalogVersion = body.CatalogVersion
		typed.Columns = researchScreenResultColumns(body.ScreenDefinitionV2)
		httpserver.WriteOK(c, typed)
	}
}

func normalizeResearchScreenQuery(query *broker.ScreenQueryV2) error {
	if query.Page.Offset < 0 {
		return errors.New("page.offset must be non-negative")
	}
	if query.Page.Limit == 0 {
		query.Page.Limit = 50
	}
	if query.Page.Limit < 1 || query.Page.Limit > 100 {
		return errors.New("page.limit must be between 1 and 100")
	}
	normalized, err := researchscreen.NormalizeDefinitionV2(query.ScreenDefinitionV2)
	if err != nil {
		return err
	}
	query.ScreenDefinitionV2 = normalized
	return nil
}

func researchScreenResultColumns(definition broker.ScreenDefinitionV2) []broker.ScreenResultColumn {
	columns := make([]broker.ScreenResultColumn, 0, len(definition.Columns))
	for _, column := range definition.Columns {
		factor, _ := researchscreen.Lookup(column.Factor.FactorKey)
		columns = append(columns, broker.ScreenResultColumn{
			ColumnID: column.ID, InstanceID: column.Factor.InstanceID,
			FactorKey: column.Factor.FactorKey, Label: column.Label, Unit: factor.Unit,
		})
	}
	return columns
}

func typedResearchScreenResult(result *broker.FeatureResult) (broker.ResearchScreenResult, error) {
	typed := broker.ResearchScreenResult{
		Entries: []broker.ResearchScreenRow{},
	}
	if result == nil {
		return typed, nil
	}
	typed.Provider = result.Provider
	typed.AsOf = result.AsOf
	typed.Warnings = append([]string(nil), result.Warnings...)
	typed.PartialErrors = append([]broker.FeaturePartialError(nil), result.PartialErrors...)
	if result.Total != nil {
		total := *result.Total
		typed.Total = &total
	}
	if result.HasMore != nil {
		typed.HasMore = *result.HasMore
	}
	if typed.HasMore {
		next, err := strconv.Atoi(result.NextCursor)
		if err != nil || next < 0 {
			return typed, errors.New("broker returned an invalid stock-screen offset")
		}
		typed.NextOffset = &next
	}
	for _, entry := range result.Entries {
		content, err := json.Marshal(entry)
		if err != nil {
			return typed, errors.New("broker returned an invalid stock-screen row")
		}
		var row broker.ResearchScreenRow
		if err := json.Unmarshal(content, &row); err != nil {
			return typed, errors.New("broker returned an invalid stock-screen row")
		}
		if row.Values == nil {
			row.Values = map[string]broker.ResearchScreenValue{}
		}
		typed.Entries = append(typed.Entries, row)
	}
	return typed, nil
}

func writeResearchScreenError(c *gin.Context, err error) {
	writeQueryError(c, err)
}
