package productfeatures

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

// embeddedScreen routes stock screen definitions that pin the embedded catalog
// version to the embedded market-data provider. Definitions pinned to the Futu
// catalog resolve to a capability-unavailable error instead of falling through
// to broker routing, so an old preset cannot silently switch providers.
func (s *Service) embeddedScreen(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	now time.Time,
) (*broker.FeatureResult, error) {
	definition, err := decodeEmbeddedScreenDefinition(query)
	if err != nil {
		return nil, err
	}
	request, err := embeddedScreenRequest(query, definition)
	if err != nil {
		return nil, err
	}
	response, err := reader.GetScreen(ctx, request)
	if err != nil {
		return nil, err
	}
	return projectProviderScreen(descriptor, query, definition, response, now), nil
}

// decodeEmbeddedScreenDefinition mirrors the Futu adapter's
// decodeResearchScreenDefinition: the query param is normally the typed
// definition placed by QueryScreen, but JSON-round-trip decoding keeps
// internal callers that hand us a map working. The catalog version gate runs
// before normalization so a Futu-pinned preset resolves to a clean
// capability-unavailable error regardless of its factor keys; re-normalization
// is idempotent because the API layer has already validated the draft.
func decodeEmbeddedScreenDefinition(query *broker.FeatureQuery) (broker.ScreenDefinitionV2, error) {
	raw, ok := query.Params["researchScreenDefinition"]
	if !ok || raw == nil {
		return broker.ScreenDefinitionV2{}, fmt.Errorf(
			"%w: stock screen definition is required", ErrCapabilityUnavailable,
		)
	}
	definition, ok := raw.(broker.ScreenDefinitionV2)
	if !ok {
		content, err := json.Marshal(raw)
		if err != nil {
			return broker.ScreenDefinitionV2{}, fmt.Errorf("encode stock screen definition: %w", err)
		}
		if err := json.Unmarshal(content, &definition); err != nil {
			return broker.ScreenDefinitionV2{}, fmt.Errorf("invalid stock screen definition: %w", err)
		}
	}
	if !researchscreen.IsEmbeddedCatalogVersion(definition.CatalogVersion) {
		return broker.ScreenDefinitionV2{}, fmt.Errorf(
			"%w: catalog %q requires the futu broker",
			ErrCapabilityUnavailable, definition.CatalogVersion,
		)
	}
	return researchscreen.NormalizeDefinitionV2(definition)
}

// embeddedScreenRequest folds the normalized definition into the
// provider-neutral screen query. Interval conditions map to min/max bounds;
// multi-interval drafts and absolute-value sorts have no provider-neutral
// form and stay capability-unavailable.
func embeddedScreenRequest(
	query *broker.FeatureQuery,
	definition broker.ScreenDefinitionV2,
) (marketdata.ScreenRequest, error) {
	request := marketdata.ScreenRequest{
		Market: definition.Market,
		Offset: embeddedScreenOffset(query),
		Limit:  embeddedScreenLimit(query),
	}
	if request.Market == "" {
		request.Market = strings.ToUpper(strings.TrimSpace(query.Market))
	}
	for index, condition := range definition.Conditions {
		converted, err := embeddedScreenCondition(condition)
		if err != nil {
			return marketdata.ScreenRequest{}, fmt.Errorf("conditions[%d]: %w", index, err)
		}
		request.Conditions = append(request.Conditions, converted)
	}
	for index, sort := range definition.Sorts {
		if sort.Direction != "asc" && sort.Direction != "desc" {
			return marketdata.ScreenRequest{}, fmt.Errorf(
				"%w: sorts[%d] direction %q is not executable against the embedded catalog",
				ErrCapabilityUnavailable, index, sort.Direction,
			)
		}
		request.Sorts = append(request.Sorts, marketdata.ScreenSortRequest{
			FactorKey: sort.Factor.FactorKey,
			Direction: sort.Direction,
		})
	}
	return request, nil
}

func embeddedScreenCondition(condition broker.ScreenCondition) (marketdata.ScreenConditionRequest, error) {
	if condition.Operator != "between" {
		return marketdata.ScreenConditionRequest{}, fmt.Errorf(
			"%w: operator %q is not executable against the embedded catalog",
			ErrCapabilityUnavailable, condition.Operator,
		)
	}
	rangeValue, ok := condition.Value.(map[string]any)
	if !ok {
		return marketdata.ScreenConditionRequest{}, fmt.Errorf(
			"%w: condition value must be an interval", ErrCapabilityUnavailable,
		)
	}
	if _, exists := rangeValue["intervals"]; exists {
		return marketdata.ScreenConditionRequest{}, fmt.Errorf(
			"%w: multi-interval conditions are not executable against the embedded catalog",
			ErrCapabilityUnavailable,
		)
	}
	converted := marketdata.ScreenConditionRequest{FactorKey: condition.Factor.FactorKey}
	if min, ok := embeddedScreenNumber(rangeValue["min"]); ok {
		converted.Min = &min
	}
	if max, ok := embeddedScreenNumber(rangeValue["max"]); ok {
		converted.Max = &max
	}
	if converted.Min == nil && converted.Max == nil {
		return marketdata.ScreenConditionRequest{}, fmt.Errorf(
			"%w: condition requires min or max", ErrCapabilityUnavailable,
		)
	}
	return converted, nil
}

// embeddedScreenNumber renders a decoded JSON number without float
// re-formatting where possible so the sidecar receives the exact bound the
// editor validated.
func embeddedScreenNumber(value any) (json.Number, bool) {
	switch typed := value.(type) {
	case json.Number:
		return typed, true
	case float64:
		return json.Number(strconv.FormatFloat(typed, 'g', -1, 64)), true
	case float32:
		return json.Number(strconv.FormatFloat(float64(typed), 'g', -1, 32)), true
	case int:
		return json.Number(strconv.Itoa(typed)), true
	case int64:
		return json.Number(strconv.FormatInt(typed, 10)), true
	default:
		return "", false
	}
}

func embeddedScreenOffset(query *broker.FeatureQuery) int {
	if offset, err := strconv.Atoi(strings.TrimSpace(query.Cursor)); err == nil && offset >= 0 {
		return offset
	}
	if pageFrom, ok := query.Params["pageFrom"]; ok {
		if offset, ok := embeddedScreenNumber(pageFrom); ok {
			if parsed, err := strconv.Atoi(offset.String()); err == nil && parsed >= 0 {
				return parsed
			}
		}
	}
	return 0
}

func embeddedScreenLimit(query *broker.FeatureQuery) int {
	limit := query.PageSize
	if limit <= 0 {
		limit = marketdata.DefaultScreenLimit
	}
	return min(max(limit, 1), marketdata.MaxScreenLimit)
}
