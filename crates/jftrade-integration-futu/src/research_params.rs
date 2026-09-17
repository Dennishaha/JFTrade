//! Advanced research/OpenD C2S parameter ownership.
//!
//! The Go adapter translated public research queries into strict OpenD
//! parameters (`injectAdvancedProtocolDefaults` in
//! `pkg/futu/adapter_advanced_defaults.go`). The Rust engine previously
//! validated only typed ports, so a public research query could reach a reader
//! with missing or mistranslated scope (market, plate type, direction,
//! institution id, news keyword). This module owns that translation once so
//! HTTP adapters and typed readers cannot silently diverge.

use serde_json::{Map, Value};

/// Error raised by parameter normalization. The messages intentionally keep
/// the Go substrings the compatibility checklist asserts on.
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ResearchParamsError {
    Invalid(String),
}

impl std::fmt::Display for ResearchParamsError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Invalid(message) => formatter.write_str(message),
        }
    }
}

impl std::error::Error for ResearchParamsError {}

impl ResearchParamsError {
    pub fn message(&self) -> &str {
        match self {
            Self::Invalid(message) => message,
        }
    }
}

/// Public query scope shared by the research adapters.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct ResearchQueryScope {
    pub market: String,
    pub instrument_id: String,
}

impl ResearchQueryScope {
    pub fn new(market: impl Into<String>, instrument_id: impl Into<String>) -> Self {
        Self {
            market: market.into(),
            instrument_id: instrument_id.into(),
        }
    }
}

fn invalid(message: impl Into<String>) -> ResearchParamsError {
    ResearchParamsError::Invalid(message.into())
}

fn string_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(value)) => value.clone(),
        Some(Value::Number(value)) => value.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        Some(Value::Null) | None => String::new(),
        Some(other) => other.to_string(),
    }
}

fn research_number(value: Option<&Value>) -> Option<f64> {
    let value = value?;
    match value {
        Value::Number(number) => number.as_f64(),
        Value::String(text) => text.trim().parse::<f64>().ok(),
        _ => None,
    }
    .filter(|number| number.is_finite())
}

/// Shared `boundedResearchEnum` semantics: integers only, inclusive bounds.
pub fn bounded_research_enum(value: &Value, minimum: i64, maximum: i64) -> Option<i64> {
    let number = research_number(Some(value))?;
    if number.fract() != 0.0 {
        return None;
    }
    let number = number as i64;
    (minimum..=maximum).contains(&number).then_some(number)
}

/// Go `researchNumber`: numeric scalars plus whitespace-padded decimal text.
pub fn research_number_parity(value: &Value) -> Option<f64> {
    research_number(Some(value))
}

fn lower_text(value: &Value) -> String {
    match value {
        Value::String(text) => text.trim().to_ascii_lowercase(),
        Value::Number(number) => number.to_string(),
        _ => String::new(),
    }
}

fn market_code(market: &str) -> Option<i32> {
    match market.trim().to_ascii_uppercase().as_str() {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" | "CN" => Some(21),
        "SZ" => Some(22),
        "SG" => Some(31),
        "JP" => Some(41),
        "AU" => Some(51),
        "MY" => Some(61),
        "CA" => Some(71),
        _ => None,
    }
}

fn macro_region(market: &str) -> i32 {
    match market.trim().to_ascii_uppercase().as_str() {
        "HK" => 1,
        "SH" | "SZ" => 8,
        _ => 2,
    }
}

/// Go `advancedPageSizeLimit`: the hard OpenD cap for a protocol's declared
/// pagination field. `Qot_GetIndustrialChainList` rejects `count` outside
/// `[1, 50]`; every other advanced protocol accepts up to 100.
pub fn advanced_page_size_limit(protocol: &str) -> i32 {
    match protocol {
        "Qot_GetIndustrialChainList" => 50,
        _ => 100,
    }
}

/// Pagination field the protocol's C2S payload actually declares.
///
/// Go resolved this by reflecting over the registered protobuf descriptors
/// (`opend.AdvancedC2SHasField`). Rust keeps the same probe order
/// (`count`, `pageCount`, `num`, `maxCount`, `maxRetNum`) and records the
/// answer for every protocol the capability catalog can send, so a request can
/// never carry a field the generated message does not declare.
pub fn advanced_page_size_field(protocol: &str) -> Option<&'static str> {
    let field = match protocol {
        "Qot_GetArkActiveTransaction"
        | "Qot_GetArkFundHolding"
        | "Qot_GetDividendCalendar"
        | "Qot_GetDividendRank"
        | "Qot_GetEarningsBeatRank"
        | "Qot_GetEconomicCalendar"
        | "Qot_GetEventContract"
        | "Qot_GetEventContractComboList"
        | "Qot_GetEventContractEventList"
        | "Qot_GetEventContractMilestoneList"
        | "Qot_GetEventContractTicker"
        | "Qot_GetHeatMapData"
        | "Qot_GetHighDividendSOERank"
        | "Qot_GetHotList"
        | "Qot_GetIndustrialChainList"
        | "Qot_GetIndustrialPlateStock"
        | "Qot_GetInstitutionHoldingChange"
        | "Qot_GetInstitutionHoldingList"
        | "Qot_GetInstitutionList"
        | "Qot_GetOptionEarningsScreener"
        | "Qot_GetOptionEvent"
        | "Qot_GetOptionEventAlert"
        | "Qot_GetOptionRank"
        | "Qot_GetOptionUnderlyingRank"
        | "Qot_GetOptionZeroDteScreener"
        | "Qot_GetPeriodChangeRank"
        | "Qot_GetRatingChange"
        | "Qot_GetShortSellingRank"
        | "Qot_GetTopMoversRank"
        | "Qot_GetUSAfterHoursRank"
        | "Qot_GetUSOvernightRank"
        | "Qot_GetUSPreMarketRank" => "count",
        "Qot_OptionScreen" | "Qot_StockScreen" | "Qot_WarrantScreen" => "pageCount",
        "Qot_GetCompanyOperationalEfficiency"
        | "Qot_GetCorporateActionsBuybacks"
        | "Qot_GetCorporateActionsStockSplits"
        | "Qot_GetDailyShortVolume"
        | "Qot_GetEventContractOrderBook"
        | "Qot_GetFinancialsStatements"
        | "Qot_GetInsiderHolderList"
        | "Qot_GetInsiderTradeList"
        | "Qot_GetOrderBook"
        | "Qot_GetResearchRatingSummary"
        | "Qot_GetShareholdersHolderDetail"
        | "Qot_GetShareholdersHoldingChanges"
        | "Qot_GetShareholdersInstitutional"
        | "Qot_GetShortInterest"
        | "Qot_GetValuationPlateStockList"
        | "Qot_GetWarrant"
        | "Qot_RequestIndicatorCalc"
        | "Qot_StockFilter" => "num",
        "Qot_GetEventContractKline"
        | "Qot_GetMacroIndicatorHistory"
        | "Qot_GetSearchNews"
        | "Qot_GetSearchQuote" => "maxCount",
        "Qot_GetTicker" => "maxRetNum",
        _ => return None,
    };
    Some(field)
}

/// Clamp a requested page size the way Go's `injectAdvancedPageSize` does:
/// non-positive requests become `1` and oversized requests stop at the
/// protocol limit. Unknown protocols keep the caller's value.
pub fn clamp_advanced_page_size(protocol: &str, page_size: i32) -> i32 {
    if advanced_page_size_field(protocol).is_none() {
        return page_size;
    }
    page_size.max(1).min(advanced_page_size_limit(protocol))
}

/// Go `injectAdvancedPageSize`: clamp the caller's page size and write it to
/// the protocol's declared pagination field. A caller that already supplied
/// the field keeps ownership, and a protocol without such a field never
/// receives one — `count` must not leak into strict OpenD validation.
pub fn inject_advanced_page_size(params: &mut Map<String, Value>, protocol: &str, page_size: i32) {
    let Some(field) = advanced_page_size_field(protocol) else {
        return;
    };
    if params.get(field).is_some_and(|value| !value.is_null()) {
        return;
    }
    params.insert(
        field.to_owned(),
        Value::from(clamp_advanced_page_size(protocol, page_size)),
    );
}

/// Cursor field the protocol's C2S payload declares, using Go's probe order
/// (`nextPage`, `page`, `nextKey`).
pub fn advanced_cursor_field(protocol: &str) -> Option<&'static str> {
    let field = match protocol {
        "Qot_GetEconomicCalendar"
        | "Qot_GetEventContract"
        | "Qot_GetEventContractComboList"
        | "Qot_GetEventContractEventList"
        | "Qot_GetEventContractMilestoneList" => "nextPage",
        "Qot_GetArkActiveTransaction"
        | "Qot_GetArkFundHolding"
        | "Qot_GetHeatMapData"
        | "Qot_GetIndustrialChainList"
        | "Qot_GetIndustrialPlateStock"
        | "Qot_GetInstitutionHoldingChange"
        | "Qot_GetInstitutionHoldingList"
        | "Qot_GetInstitutionList"
        | "Qot_GetOptionEarningsScreener"
        | "Qot_GetOptionEvent"
        | "Qot_GetOptionEventAlert"
        | "Qot_GetOptionRank"
        | "Qot_GetOptionUnderlyingRank"
        | "Qot_GetOptionZeroDteScreener"
        | "Qot_GetRatingChange" => "page",
        "Qot_GetCompanyOperationalEfficiency"
        | "Qot_GetCorporateActionsBuybacks"
        | "Qot_GetCorporateActionsStockSplits"
        | "Qot_GetDailyShortVolume"
        | "Qot_GetFinancialsStatements"
        | "Qot_GetInsiderHolderList"
        | "Qot_GetInsiderTradeList"
        | "Qot_GetResearchRatingSummary"
        | "Qot_GetShareholdersHolderDetail"
        | "Qot_GetShareholdersHoldingChanges"
        | "Qot_GetShareholdersInstitutional"
        | "Qot_GetShortInterest"
        | "Qot_GetValuationPlateStockList" => "nextKey",
        _ => return None,
    };
    Some(field)
}

/// Go `injectAdvancedCursor`: write the cursor into the protocol's declared
/// cursor field, leaving a caller-owned value untouched. A protocol that
/// declares no cursor field simply does not paginate on the wire.
pub fn inject_advanced_cursor(params: &mut Map<String, Value>, protocol: &str, cursor: &str) {
    let Some(field) = advanced_cursor_field(protocol) else {
        return;
    };
    if cursor.trim().is_empty() || params.get(field).is_some_and(|value| !value.is_null()) {
        return;
    }
    params.insert(field.to_owned(), Value::from(cursor.trim()));
}

/// Go `injectAdvancedDefaults`: scope defaults shared by every advanced
/// protocol. `market` is translated from the public market label, `offset` and
/// `pageFrom` start at zero when the protocol declares them, and the
/// protocol-specific rules are applied on top.
pub fn inject_advanced_defaults(
    params: &mut Map<String, Value>,
    protocol: &str,
    scope: &ResearchQueryScope,
) -> Result<(), ResearchParamsError> {
    if advanced_has_market_field(protocol) && params.get("market").is_none_or(Value::is_null) {
        // Go translates the public market label and only surfaces the lookup
        // failure when the caller supplied one; an empty scope keeps the
        // historic unknown-market code instead of failing the request.
        let market = match market_code(&scope.market) {
            Some(market) => market,
            None if scope.market.trim().is_empty() => 0,
            None => {
                return Err(invalid(format!(
                    "futu: unsupported market {:?}",
                    scope.market.trim()
                )));
            }
        };
        params.insert("market".to_owned(), Value::from(market));
    }
    if advanced_has_field(protocol, "offset") && params.get("offset").is_none_or(Value::is_null) {
        params.insert("offset".to_owned(), Value::from(0));
    }
    if advanced_has_field(protocol, "pageFrom") && params.get("pageFrom").is_none_or(Value::is_null)
    {
        params.insert("pageFrom".to_owned(), Value::from(0));
    }
    inject_advanced_protocol_defaults(params, protocol, scope)
}

/// Protocols whose C2S payload declares a `market` field, so the public market
/// label can be translated into the OpenD market code.
pub fn advanced_has_market_field(protocol: &str) -> bool {
    advanced_has_field(protocol, "market")
}

/// Whether the protocol's C2S payload declares `field`. Go reflected over the
/// registered descriptors; Rust records the fields the catalog can send.
pub fn advanced_has_field(protocol: &str, field: &str) -> bool {
    match field {
        "market" => matches!(
            protocol,
            "Qot_GetDividendCalendar"
                | "Qot_GetDividendRank"
                | "Qot_GetEarningsBeatRank"
                | "Qot_GetEarningsCalendar"
                | "Qot_GetHeatMapData"
                | "Qot_GetHotList"
                | "Qot_GetIndustrialChainList"
                | "Qot_GetInstitutionDistribution"
                | "Qot_GetInstitutionHoldingChange"
                | "Qot_GetInstitutionHoldingList"
                | "Qot_GetInstitutionList"
                | "Qot_GetInstitutionProfile"
                | "Qot_GetIpoList"
                | "Qot_GetPeriodChangeRank"
                | "Qot_GetPlateSet"
                | "Qot_GetPriceReminder"
                | "Qot_GetRatingChange"
                | "Qot_GetRiseFallDistribution"
                | "Qot_GetShortSellingRank"
                | "Qot_GetStaticInfo"
                | "Qot_GetTopMoversRank"
                | "Qot_RequestTradeDate"
                | "Qot_StockFilter"
        ),
        "offset" => matches!(
            protocol,
            "Qot_GetHighDividendSOERank"
                | "Qot_GetHotList"
                | "Qot_GetPeriodChangeRank"
                | "Qot_GetShortSellingRank"
                | "Qot_GetTopMoversRank"
                | "Qot_GetUSAfterHoursRank"
                | "Qot_GetUSOvernightRank"
                | "Qot_GetUSPreMarketRank"
        ),
        "pageFrom" => matches!(
            protocol,
            "Qot_OptionScreen" | "Qot_StockScreen" | "Qot_WarrantScreen"
        ),
        _ => false,
    }
}

/// Go `advancedProtocolReplaySafe` defaults to *no* automatic replay.
///
/// Only read protocols on the allowlist may be retried after a reconnect.
/// `Qot_GetEventContractComboRfq` keeps its `Get` prefix but creates a
/// short-lived quote, so duplicating it when the first outcome is unknown is
/// unsafe; alerts, reminders and watchlist writes stay single-attempt too.
pub fn advanced_protocol_replay_safe(protocol: &str) -> bool {
    if protocol == "Qot_GetEventContractComboRfq" {
        return false;
    }
    if protocol.starts_with("Qot_Get")
        || protocol.starts_with("Qot_Request")
        || protocol.starts_with("Qot_Filter")
    {
        return true;
    }
    matches!(
        protocol,
        "Qot_OptionScreen"
            | "Qot_WarrantScreen"
            | "Qot_StockFilter"
            | "Qot_StockScreen"
            | "Qot_SubEventContract"
    )
}

/// Go `translateTopMoversDirection`: empty means "no filter", `up`/`down`
/// map to `sortDir`, everything else is rejected.
pub fn translate_top_movers_direction(
    params: &mut Map<String, Value>,
) -> Result<(), ResearchParamsError> {
    let value = params
        .remove("direction")
        .map(|value| lower_text(&value))
        .unwrap_or_default();
    if value.is_empty() {
        return Ok(());
    }
    match value.as_str() {
        "up" => {
            params.insert("sortDir".to_owned(), Value::from(0));
            Ok(())
        }
        "down" => {
            params.insert("sortDir".to_owned(), Value::from(1));
            Ok(())
        }
        _ => Err(invalid(format!(
            "futu: unsupported top movers direction {value:?}"
        ))),
    }
}

/// Go `translateHeatMapPlateType`: accept 0..=2 or the three named plate
/// scopes; a missing value is left untouched. NaN/fractional values are
/// rejected exactly like Go.
pub fn translate_heat_map_plate_type(
    params: &mut Map<String, Value>,
) -> Result<(), ResearchParamsError> {
    let Some(value) = params.get("plateType").cloned() else {
        return Ok(());
    };
    if !matches!(&value, Value::String(text) if !text.trim().is_empty())
        && let Some(number) = bounded_research_enum(&value, 0, 2)
    {
        params.insert("plateType".to_owned(), Value::from(number));
        return Ok(());
    }
    let text = lower_text(&value);
    let mapped = match text.as_str() {
        "industry" => 0,
        "concept" => 1,
        "theme" => 2,
        _ => {
            return Err(invalid(format!(
                "futu: unsupported heatmap plateType {value}"
            )));
        }
    };
    params.insert("plateType".to_owned(), Value::from(mapped));
    Ok(())
}

/// Go `translatePlateSetType`: `plateType` is the legacy alias and is always
/// removed from the wire request.
pub fn translate_plate_set_type(
    params: &mut Map<String, Value>,
) -> Result<(), ResearchParamsError> {
    let value = params
        .remove("plateSetType")
        .or_else(|| params.remove("plateType"));
    let Some(value) = value else {
        return Err(invalid("futu: plate_list requires plateType"));
    };
    if let Some(number) = bounded_research_enum(&value, 0, 3) {
        params.insert("plateSetType".to_owned(), Value::from(number));
        return Ok(());
    }
    let text = lower_text(&value);
    let mapped = match text.as_str() {
        "all" => 0,
        "industry" => 1,
        "region" => 2,
        "concept" => 3,
        _ => {
            return Err(invalid(format!("futu: unsupported plateType {value}")));
        }
    };
    params.insert("plateSetType".to_owned(), Value::from(mapped));
    Ok(())
}

fn inject_option_chain_dates(params: &mut Map<String, Value>) {
    let now = time::OffsetDateTime::now_utc();
    if params.get("beginTime").is_none_or(Value::is_null) {
        params.insert("beginTime".to_owned(), Value::from(format_date(now)));
    }
    if params.get("endTime").is_none_or(Value::is_null) {
        params.insert(
            "endTime".to_owned(),
            Value::from(format_date(now + time::Duration::days(30))),
        );
    }
}

fn inject_option_market_statistic_defaults(params: &mut Map<String, Value>, market: &str) {
    let now = time::OffsetDateTime::now_utc();
    if params.get("optionMarket").is_none_or(Value::is_null) {
        let value = if market.trim().eq_ignore_ascii_case("HK") {
            3
        } else {
            1
        };
        params.insert("optionMarket".to_owned(), Value::from(value));
    }
    if params.get("dataType").is_none_or(Value::is_null) {
        params.insert("dataType".to_owned(), Value::from(0));
    }
    if params.get("beginTime").is_none_or(Value::is_null) {
        params.insert(
            "beginTime".to_owned(),
            Value::from(format_date(now - time::Duration::days(30))),
        );
    }
    if params.get("endTime").is_none_or(Value::is_null) {
        params.insert("endTime".to_owned(), Value::from(format_date(now)));
    }
}

fn inject_historical_date_range(params: &mut Map<String, Value>) {
    let now = time::OffsetDateTime::now_utc();
    if params.get("beginTime").is_none_or(Value::is_null) {
        params.insert(
            "beginTime".to_owned(),
            Value::from(format_date(now - time::Duration::days(365))),
        );
    }
    if params.get("endTime").is_none_or(Value::is_null) {
        params.insert("endTime".to_owned(), Value::from(format_date(now)));
    }
}

fn inject_warrant_defaults(params: &mut Map<String, Value>) {
    if params.get("begin").is_none_or(Value::is_null) {
        params.insert("begin".to_owned(), Value::from(0));
    }
    if params.get("sortField").is_none_or(Value::is_null) {
        params.insert("sortField".to_owned(), Value::from(12));
    }
    if params.get("ascend").is_none_or(Value::is_null) {
        params.insert("ascend".to_owned(), Value::from(false));
    }
}

fn format_date(value: time::OffsetDateTime) -> String {
    format!(
        "{:04}-{:02}-{:02}",
        value.year(),
        value.month() as u8,
        value.day()
    )
}

/// Apply every Go `injectAdvancedProtocolDefaults` option rule.
pub fn inject_advanced_option_defaults(
    params: &mut Map<String, Value>,
    protocol: &str,
    scope: &ResearchQueryScope,
) -> Result<(), ResearchParamsError> {
    match protocol {
        "Qot_GetOptionChain" => inject_option_chain_dates(params),
        "Qot_OptionScreen" => {
            if params.get("marketCategoryList").is_none_or(Value::is_null) {
                let category = if scope.market.trim().eq_ignore_ascii_case("HK") {
                    3
                } else {
                    0
                };
                params.insert(
                    "marketCategoryList".to_owned(),
                    Value::Array(vec![Value::from(category)]),
                );
            }
        }
        "Qot_GetOptionMarketStatistic" => {
            inject_option_market_statistic_defaults(params, &scope.market)
        }
        "Qot_GetOptionUnderlyingHisStatistic" | "Qot_GetOptionUnderlyingHisVolatility" => {
            inject_historical_date_range(params);
        }
        "Qot_GetWarrant" => inject_warrant_defaults(params),
        "Qot_WarrantScreen" => {
            if params.get("marketType").is_none_or(Value::is_null) {
                params.insert("marketType".to_owned(), Value::from(1));
            }
        }
        "Qot_GetMacroIndicatorList" => {
            if params.get("region").is_none_or(Value::is_null) {
                params.insert(
                    "region".to_owned(),
                    Value::from(macro_region(&scope.market)),
                );
            }
        }
        "Qot_GetTopMoversRank" => translate_top_movers_direction(params)?,
        "Qot_GetHighDividendSOERank" if !scope.market.trim().eq_ignore_ascii_case("HK") => {
            return Err(invalid(
                "futu: high_dividend_state is available only for HK; OpenD does not accept a market filter",
            ));
        }
        _ => {}
    }
    Ok(())
}

/// Apply every Go `injectAdvancedResearchDefaults` rule.
pub fn inject_advanced_research_defaults(
    params: &mut Map<String, Value>,
    protocol: &str,
    scope: &ResearchQueryScope,
) -> Result<(), ResearchParamsError> {
    match protocol {
        "Qot_GetHeatMapData" => translate_heat_map_plate_type(params)?,
        "Qot_GetPlateSet" => {
            translate_plate_set_type(params)?;
            if scope.market.trim().is_empty() || params.get("market").is_none_or(Value::is_null) {
                return Err(invalid("futu: plate_list requires market"));
            }
        }
        "Qot_GetPlateSecurity" => {
            if params.get("plate").is_none_or(Value::is_null) {
                return Err(invalid(
                    "futu: plate_members requires an exact plate instrumentId",
                ));
            }
        }
        "Qot_GetStaticInfo" => {
            if scope.market.trim().is_empty() || params.get("market").is_none_or(Value::is_null) {
                return Err(invalid("futu: fund_catalog requires market"));
            }
            params.insert("secType".to_owned(), Value::from(4));
        }
        "Qot_GetEconomicCalendar" => {
            if string_value(params.get("beginDate")).trim().is_empty() {
                return Err(invalid("futu: economic calendar requires beginDate"));
            }
            if params.get("marketList").is_none_or(Value::is_null)
                && !scope.market.trim().is_empty()
            {
                let market = market_code(&scope.market).ok_or_else(|| {
                    invalid(format!(
                        "futu: unsupported market {:?}",
                        scope.market.trim()
                    ))
                })?;
                params.insert(
                    "marketList".to_owned(),
                    Value::Array(vec![Value::from(market)]),
                );
            }
        }
        "Qot_GetDividendCalendar" => {
            if string_value(params.get("date")).trim().is_empty() {
                return Err(invalid("futu: dividend calendar requires date"));
            }
        }
        "Qot_GetInstitutionProfile"
        | "Qot_GetInstitutionDistribution"
        | "Qot_GetInstitutionHoldingChange"
        | "Qot_GetInstitutionHoldingList" => {
            let number = research_number(params.get("institutionId"));
            let valid = number.is_some_and(|number| {
                number > 0.0 && number <= f64::from(i32::MAX) && number.fract() == 0.0
            });
            if !valid {
                return Err(invalid(format!(
                    "futu: {protocol} requires a positive integer institutionId"
                )));
            }
            params.insert(
                "institutionId".to_owned(),
                Value::from(number.unwrap_or_default() as i32),
            );
        }
        "Qot_GetSearchNews" => {
            if string_value(params.get("keyword")).trim().is_empty() {
                let quoted = format!("{}.", scope.market.trim());
                let keyword = scope
                    .instrument_id
                    .strip_prefix(quoted.as_str())
                    .unwrap_or(scope.instrument_id.as_str())
                    .trim();
                if keyword.is_empty() {
                    return Err(invalid(
                        "futu: news search requires keyword or instrumentId",
                    ));
                }
                params.insert("keyword".to_owned(), Value::from(keyword));
            }
        }
        "Qot_GetEventContractKline" if params.get("klineSource").is_none_or(Value::is_null) => {
            params.insert("klineSource".to_owned(), Value::from(1));
        }
        _ => {}
    }
    Ok(())
}

/// Convenience wrapper mirroring Go `injectAdvancedProtocolDefaults`.
pub fn inject_advanced_protocol_defaults(
    params: &mut Map<String, Value>,
    protocol: &str,
    scope: &ResearchQueryScope,
) -> Result<(), ResearchParamsError> {
    inject_advanced_option_defaults(params, protocol, scope)?;
    inject_advanced_research_defaults(params, protocol, scope)
}

/// Test helper that builds a params map from key/value pairs.
#[cfg(test)]
pub fn params_from_pairs(
    pairs: impl IntoIterator<Item = (&'static str, Value)>,
) -> Map<String, Value> {
    let mut params = Map::new();
    for (key, value) in pairs {
        params.insert(key.to_owned(), value);
    }
    params
}

#[cfg(test)]
#[path = "research_params_tests.rs"]
mod tests;
