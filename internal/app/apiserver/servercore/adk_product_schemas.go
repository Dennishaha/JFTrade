package servercore

import "maps"

import "strings"

var productMarketEnum = []string{"HK", "US", "SH", "SZ"}

func productToolInputSchema(name string) map[string]any {
	switch strings.TrimSpace(name) {
	case "market.capabilities":
		return objectSchema(commonCapabilityProperties(), nil)
	case "market.search":
		return objectSchema(readProperties(
			map[string]any{"query": stringSchema(1, 120)},
		), []string{"query"})
	case "market.snapshot":
		return objectSchema(readProperties(instrumentProperties()), []string{"instrumentId"})
	case "market.snapshots":
		return objectSchema(readProperties(map[string]any{
			"symbols": map[string]any{
				"type": "array", "items": stringSchema(1, 80),
				"minItems": 1, "maxItems": 200,
			},
		}), []string{"symbols"})
	case "market.candles":
		return marketSeriesToolSchema(true)
	case "market.depth":
		return marketSeriesToolSchema(false)
	case "market.instrument_profile", "market.intraday", "market.ticks",
		"market.broker_queue", "market.capital_flow", "derivatives.option_chain",
		"derivatives.option_analysis", "research.instrument", "research.financials",
		"research.valuation", "research.analyst", "research.ownership",
		"research.corporate_actions", "research.short_interest", "research.news",
		"research.technical_indicators", "prediction.snapshot", "prediction.depth",
		"prediction.history":
		return objectSchema(readProperties(instrumentOperationProperties(name)), []string{"instrumentId"})
	case "derivatives.option_screen", "derivatives.option_events", "derivatives.warrants",
		"derivatives.futures", "research.screen", "research.calendar", "research.macro",
		"research.rankings", "research.institutions", "research.industry",
		"prediction.combo_eligible", "alerts.price.list", "alerts.option_event.list",
		"watchlist.remote.list":
		return objectSchema(readProperties(operationProperties(name)), nil)
	case "prediction.discover":
		return objectSchema(readProperties(predictionDiscoveryProperties()), []string{"operation"})
	case "prediction.combo_quote":
		return objectSchema(readProperties(predictionQuoteProperties()), []string{"accountId", "mvc", "legs"})
	case "execution.buying_power":
		return objectSchema(productRuleProperties(), []string{
			"accountId", "tradingEnvironment", "market", "instrument", "orderKind",
		})
	case "execution.order_preview":
		return objectSchema(singleOrderProperties(false), []string{
			"brokerId", "accountId", "tradingEnvironment", "market",
			"clientOrderId", "orderKind", "productClass",
		})
	case "execution.order_place":
		return objectSchema(singleOrderProperties(true), []string{
			"brokerId", "accountId", "tradingEnvironment", "market",
			"clientOrderId", "previewId", "orderKind", "productClass",
		})
	case "execution.order_cancel", "execution.combo_cancel":
		return objectSchema(map[string]any{
			"internalOrderId": stringSchema(1, 120),
		}, []string{"internalOrderId"})
	case "execution.combo_preview":
		return comboToolSchema(false)
	case "execution.combo_place":
		return comboToolSchema(true)
	case "alerts.price.set", "alerts.option_event.set", "watchlist.remote.modify":
		return objectSchema(writeCustomizationProperties(name), []string{"brokerId", "payload"})
	default:
		return objectSchema(readProperties(nil), nil)
	}
}

func marketSeriesToolSchema(candles bool) map[string]any {
	properties := readProperties(map[string]any{
		"instrumentId": stringSchema(3, 80),
		"symbol":       stringSchema(1, 80),
	})
	if candles {
		properties["operation"] = enumSchema("current", "historical")
		properties["period"] = stringSchema(1, 20)
		properties["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 500}
		properties["startTime"] = stringSchema(1, 40)
		properties["endTime"] = stringSchema(1, 40)
	} else {
		properties["num"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 50}
	}
	schema := objectSchema(properties, nil)
	schema["anyOf"] = []any{
		map[string]any{"required": []string{"instrumentId"}},
		map[string]any{"required": []string{"market", "symbol"}},
	}
	return schema
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema(minLength, maxLength int) map[string]any {
	return map[string]any{
		"type": "string", "minLength": minLength, "maxLength": maxLength,
	}
}

func commonCapabilityProperties() map[string]any {
	return map[string]any{
		"brokerId":           stringSchema(1, 64),
		"accountId":          stringSchema(1, 128),
		"tradingEnvironment": enumSchema("SIMULATE", "REAL"),
		"market":             enumSchema(productMarketEnum...),
		"featureId":          stringSchema(1, 100),
	}
}

func readProperties(extra map[string]any) map[string]any {
	properties := commonCapabilityProperties()
	properties["cursor"] = stringSchema(1, 512)
	properties["pageSize"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 100}
	properties["refresh"] = map[string]any{"type": "boolean"}
	maps.Copy(properties, extra)
	return properties
}

func instrumentProperties() map[string]any {
	return map[string]any{
		"instrumentId": stringSchema(3, 80),
	}
}

func instrumentOperationProperties(name string) map[string]any {
	properties := instrumentProperties()
	if values := productToolOperations[name]; len(values) > 0 {
		properties["operation"] = enumSchema(values...)
	}
	properties["startTime"] = stringSchema(1, 40)
	properties["endTime"] = stringSchema(1, 40)
	properties["period"] = stringSchema(1, 20)
	return properties
}

func operationProperties(name string) map[string]any {
	properties := map[string]any{}
	if values := productToolOperations[name]; len(values) > 0 {
		properties["operation"] = enumSchema(values...)
	}
	properties["instrumentId"] = stringSchema(3, 80)
	properties["underlying"] = stringSchema(3, 80)
	switch name {
	case "research.rankings":
		properties["direction"] = enumSchema("up", "down")
		properties["plateType"] = enumSchema("industry", "concept", "theme")
	case "research.calendar":
		properties["beginDate"] = stringSchema(10, 10)
		properties["endDate"] = stringSchema(10, 10)
		properties["date"] = stringSchema(10, 10)
	case "research.institutions":
		properties["institutionId"] = map[string]any{"type": "integer", "minimum": 1}
	case "research.industry":
		properties["plateType"] = enumSchema("all", "industry", "concept", "region")
		properties["plateSetType"] = enumSchema("all", "industry", "concept", "region")
		properties["chainId"] = map[string]any{"type": "integer", "minimum": 1}
		properties["plateId"] = map[string]any{"type": "integer", "minimum": 1}
	}
	return properties
}

func predictionDiscoveryProperties() map[string]any {
	properties := operationProperties("prediction.discover")
	properties["category"] = stringSchema(1, 120)
	properties["tag"] = stringSchema(1, 120)
	properties["seriesId"] = stringSchema(1, 120)
	properties["eventId"] = stringSchema(1, 120)
	return properties
}

func predictionQuoteProperties() map[string]any {
	properties := map[string]any{
		"brokerId":           stringSchema(1, 64),
		"accountId":          stringSchema(1, 128),
		"tradingEnvironment": enumSchema("SIMULATE", "REAL"),
		"market":             enumSchema("US"),
		"mvc":                stringSchema(1, 160),
		"legs": map[string]any{
			"type": "array", "minItems": 2, "maxItems": 20,
			"items": eventLegSchema(),
		},
	}
	return properties
}

func productRuleProperties() map[string]any {
	return map[string]any{
		"brokerId":           stringSchema(1, 64),
		"accountId":          stringSchema(1, 128),
		"tradingEnvironment": enumSchema("SIMULATE", "REAL"),
		"market":             enumSchema(productMarketEnum...),
		"featureId":          enumSchema("execution.buying_power"),
		"instrument":         instrumentSchema(),
		"orderKind":          enumSchema("single", "option_combo", "event_single", "event_parlay"),
		"orderType":          stringSchema(1, 40),
		"session":            stringSchema(1, 40),
		"quantity":           positiveNumberSchema(),
		"amount":             positiveNumberSchema(),
		"price":              positiveNumberSchema(),
		"legs":               orderLegsSchema(),
	}
}

func singleOrderProperties(place bool) map[string]any {
	properties := comboOrderProperties(place)
	properties["code"] = stringSchema(1, 80)
	properties["symbol"] = stringSchema(1, 80)
	properties["side"] = enumSchema("BUY", "SELL", "SELL_SHORT", "BUY_BACK")
	properties["orderType"] = stringSchema(1, 40)
	properties["timeInForce"] = stringSchema(1, 40)
	properties["session"] = stringSchema(1, 40)
	properties["quantity"] = positiveNumberSchema()
	properties["quantityMode"] = enumSchema("units", "contracts", "amount")
	properties["predictionSide"] = enumSchema("YES", "NO")
	properties["stopPrice"] = positiveNumberSchema()
	properties["remark"] = stringSchema(0, 200)
	return properties
}

func comboOrderProperties(place bool) map[string]any {
	properties := map[string]any{
		"brokerId":               stringSchema(1, 64),
		"accountId":              stringSchema(1, 128),
		"tradingEnvironment":     enumSchema("SIMULATE", "REAL"),
		"market":                 enumSchema(productMarketEnum...),
		"clientOrderId":          stringSchema(1, 120),
		"orderKind":              enumSchema("single", "option_combo", "event_single", "event_parlay"),
		"productClass":           enumSchema("equity", "fund", "option", "warrant", "cbbc", "future", "event_contract"),
		"rfqId":                  stringSchema(1, 160),
		"mvc":                    stringSchema(1, 160),
		"underlyingInstrumentId": stringSchema(3, 80),
		"optionStrategy": enumSchema(
			"vertical", "straddle", "strangle", "calendar", "butterfly",
		),
		"nearExpiry": stringSchema(10, 10),
		"farExpiry":  stringSchema(10, 10),
		"spread":     positiveNumberSchema(),
		"amount":     positiveNumberSchema(),
		"price":      positiveNumberSchema(),
		"legs":       orderLegsSchema(),
	}
	if place {
		properties["previewId"] = stringSchema(1, 120)
	}
	return properties
}

func comboToolSchema(place bool) map[string]any {
	required := []string{
		"brokerId", "accountId", "tradingEnvironment", "market",
		"clientOrderId", "orderKind", "productClass", "legs",
	}
	if place {
		required = append(required, "previewId")
	}
	schema := objectSchema(comboOrderProperties(place), required)
	schema["allOf"] = []any{
		map[string]any{
			"if": map[string]any{
				"properties": map[string]any{"orderKind": map[string]any{"const": "option_combo"}},
				"required":   []string{"orderKind"},
			},
			"then": map[string]any{
				"required": []string{"underlyingInstrumentId", "optionStrategy", "nearExpiry"},
			},
		},
		map[string]any{
			"if": map[string]any{
				"properties": map[string]any{"orderKind": map[string]any{"const": "event_parlay"}},
				"required":   []string{"orderKind"},
			},
			"then": map[string]any{"required": []string{"rfqId", "mvc", "amount"}},
		},
	}
	return schema
}

func writeCustomizationProperties(name string) map[string]any {
	properties := commonCapabilityProperties()
	properties["payload"] = map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"minProperties":        1,
	}
	if strings.HasPrefix(name, "alerts.") {
		properties["instrumentId"] = stringSchema(3, 80)
	}
	return properties
}

func instrumentSchema() map[string]any {
	return objectSchema(map[string]any{
		"instrumentId":  stringSchema(3, 80),
		"code":          stringSchema(1, 80),
		"productClass":  enumSchema("equity", "fund", "option", "warrant", "cbbc", "future", "event_contract", "index", "bond"),
		"marketSegment": enumSchema("securities", "derivatives", "prediction"),
		"quoteMarket":   enumSchema(productMarketEnum...),
		"tradeMarket":   enumSchema(productMarketEnum...),
		"quantityMode":  enumSchema("units", "contracts", "amount"),
	}, []string{"instrumentId", "productClass", "marketSegment", "quoteMarket", "quantityMode"})
}

func orderLegsSchema() map[string]any {
	return map[string]any{
		"type": "array", "minItems": 1, "maxItems": 20,
		"items": orderLegSchema(),
	}
}

func orderLegSchema() map[string]any {
	return objectSchema(map[string]any{
		"instrumentId":   stringSchema(3, 80),
		"productClass":   enumSchema("option", "event_contract"),
		"side":           enumSchema("BUY", "SELL"),
		"ratio":          map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
		"quantity":       positiveNumberSchema(),
		"amount":         positiveNumberSchema(),
		"price":          positiveNumberSchema(),
		"predictionSide": enumSchema("YES", "NO"),
	}, []string{"instrumentId", "productClass", "side", "ratio"})
}

func eventLegSchema() map[string]any {
	return objectSchema(map[string]any{
		"instrumentId":   stringSchema(3, 80),
		"predictionSide": enumSchema("YES", "NO"),
		"side":           enumSchema("BUY"),
		"ratio":          map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
	}, []string{"instrumentId", "predictionSide"})
}

func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func positiveNumberSchema() map[string]any {
	return map[string]any{"type": "number", "exclusiveMinimum": 0}
}

var productToolOperations = map[string][]string{
	"market.candles":                {"current", "historical"},
	"market.depth":                  {"depth"},
	"market.capital_flow":           {"flow", "distribution"},
	"derivatives.option_chain":      {"expirations", "chain"},
	"derivatives.option_analysis":   {"quote", "volatility", "exercise_probability", "strategy", "strategy_analysis", "strategy_spread", "market_statistics", "historical_statistics", "underlying_overview", "historical_volatility", "underlying_rank", "contract_rank"},
	"derivatives.option_events":     {"unusual", "zero_dte", "zero_dte_contract", "earnings", "seller"},
	"derivatives.warrants":          {"related", "list", "screen"},
	"research.instrument":           {"profile", "executives", "executive_background", "operational_efficiency", "top_brokers"},
	"research.financials":           {"statements", "revenue_breakdown", "earnings_price_move", "earnings_price_history"},
	"research.valuation":            {"detail", "constituents"},
	"research.analyst":              {"consensus", "ratings", "morningstar", "changes"},
	"research.ownership":            {"overview", "changes", "holders", "institutional", "insider_holders", "insider_transactions", "management_changes"},
	"research.corporate_actions":    {"dividends", "buybacks", "splits", "code_changes"},
	"research.short_interest":       {"daily_volume", "short_interest"},
	"research.screen":               {"stock_v1", "stock_v2"},
	"research.calendar":             {"earnings", "dividends", "economic", "ipos", "trade_dates"},
	"research.macro":                {"indicators", "indicator_history", "fed_target_rate", "fed_dot_plot"},
	"research.rankings":             {"earnings_beat", "dividend", "pre_market", "after_hours", "overnight", "top_movers", "hot", "short_selling", "period_change", "high_dividend_state", "heatmap", "rise_fall_distribution", "market_state", "fund_catalog"},
	"research.institutions":         {"list", "profile", "distribution", "holding_changes", "holdings", "ark_fund_holdings", "ark_stock_activity", "ark_transactions"},
	"research.industry":             {"chains", "chain_detail", "chains_by_plate", "plate", "plate_stocks", "owner_plates", "plate_list", "plate_members"},
	"research.technical_indicators": {"list", "calculate"},
	"prediction.discover":           {"categories", "competitions", "series", "events", "contracts", "milestones"},
	"prediction.history":            {"candles", "historical", "ticks"},
}
