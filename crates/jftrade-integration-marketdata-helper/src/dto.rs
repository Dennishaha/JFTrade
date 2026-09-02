use std::fmt;

use serde::de;
use serde::{Deserialize, Deserializer, Serialize, Serializer};

fn deserialize_vec_or_null<'de, D, T>(deserializer: D) -> Result<Vec<T>, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de>,
{
    Ok(Option::<Vec<T>>::deserialize(deserializer)?.unwrap_or_default())
}

/// A decimal or integer value deserialized from either JSON number or JSON string.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HelperPriceValue(pub String);

impl HelperPriceValue {
    pub fn as_str(&self) -> &str {
        &self.0
    }

    pub fn to_f64(&self) -> Option<f64> {
        self.0.parse::<f64>().ok()
    }
}

impl fmt::Display for HelperPriceValue {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl Serialize for HelperPriceValue {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        serializer.serialize_str(&self.0)
    }
}

impl<'de> Deserialize<'de> for HelperPriceValue {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let value = serde_json::Value::deserialize(deserializer)?;
        match value {
            serde_json::Value::Number(num) => {
                if let Some(f) = num.as_f64()
                    && !f.is_finite()
                {
                    return Err(de::Error::custom("expected finite number"));
                }
                Ok(HelperPriceValue(num.to_string()))
            }
            serde_json::Value::String(s) => {
                let trimmed = s.trim();
                if trimmed.is_empty() {
                    return Err(de::Error::custom("price string cannot be empty"));
                }
                let parsed = trimmed.parse::<f64>().map_err(de::Error::custom)?;
                if !parsed.is_finite() {
                    return Err(de::Error::custom("expected finite number string"));
                }
                Ok(HelperPriceValue(trimmed.to_owned()))
            }
            _ => Err(de::Error::custom(
                "expected a number or string representing a decimal or integer",
            )),
        }
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperTradingWindow {
    #[serde(alias = "start_minute")]
    pub start_minute: i32,
    #[serde(alias = "end_minute")]
    pub end_minute: i32,
    pub label: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperMarketPrecision {
    pub price: i32,
    pub quote: i32,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperMarketProfile {
    pub code: String,
    #[serde(alias = "resolved_market")]
    pub resolved_market: String,
    #[serde(alias = "preferred_prefix")]
    pub preferred_prefix: String,
    #[serde(alias = "display_name")]
    pub display_name: String,
    #[serde(alias = "quote_currency")]
    pub quote_currency: String,
    pub timezone: String,
    #[serde(alias = "supports_extended_hours")]
    pub supports_extended_hours: bool,
    #[serde(alias = "requires_exchange_prefix")]
    pub requires_exchange_prefix: bool,
    #[serde(default, deserialize_with = "deserialize_vec_or_null")]
    pub aliases: Vec<String>,
    #[serde(default, deserialize_with = "deserialize_vec_or_null")]
    #[serde(alias = "regular_sessions")]
    pub regular_sessions: Vec<HelperTradingWindow>,
    pub precision: HelperMarketPrecision,
    #[serde(alias = "tick_size")]
    pub tick_size: f64,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperMarketsResponse {
    #[serde(default, deserialize_with = "deserialize_vec_or_null")]
    pub markets: Vec<HelperMarketProfile>,
    #[serde(alias = "default_market")]
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub default_market: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSearchEntry {
    pub market: String,
    #[serde(alias = "resolved_market")]
    pub resolved_market: String,
    #[serde(alias = "instrument_id")]
    pub instrument_id: String,
    pub code: String,
    pub symbol: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "security_type")]
    pub security_type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exchange: Option<String>,
    pub selectable: bool,
    pub source: String,
    #[serde(default, deserialize_with = "deserialize_vec_or_null")]
    #[serde(alias = "supported_periods")]
    pub supported_periods: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSearchResponse {
    #[serde(default, deserialize_with = "deserialize_vec_or_null")]
    pub entries: Vec<HelperSearchEntry>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSecurityResponse {
    pub market: String,
    pub symbol: String,
    #[serde(alias = "instrument_id")]
    pub instrument_id: String,
    pub name: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exchange: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub currency: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub timezone: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "security_type")]
    pub security_type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub industry: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sector: Option<String>,
    #[serde(default, deserialize_with = "deserialize_vec_or_null")]
    #[serde(alias = "supported_periods")]
    pub supported_periods: Vec<String>,
    pub source: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSnapshotQuote {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "high_price")]
    pub high_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "low_price")]
    pub low_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub volume: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub turnover: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "change_value")]
    pub change_value: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "change_rate")]
    pub change_rate: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "quote_at")]
    pub quote_at: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSnapshotResponse {
    pub market: String,
    pub symbol: String,
    #[serde(alias = "instrument_id")]
    pub instrument_id: String,
    pub price: HelperPriceValue,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bid: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ask: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "open_price")]
    pub open_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "high_price")]
    pub high_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "low_price")]
    pub low_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "previous_close_price")]
    pub previous_close_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "last_close_price")]
    pub last_close_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "regular_quote")]
    pub regular_quote: Option<HelperSnapshotQuote>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "pre_market_quote")]
    pub pre_market_quote: Option<HelperSnapshotQuote>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "after_market_quote")]
    pub after_market_quote: Option<HelperSnapshotQuote>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub volume: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub turnover: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "quote_at")]
    pub quote_at: Option<String>,
    #[serde(alias = "observed_at")]
    pub observed_at: String,
    pub source: String,
    #[serde(default)]
    pub delayed: bool,
    #[serde(default)]
    #[serde(alias = "delay_minutes")]
    pub delay_minutes: i32,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub currency: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exchange: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperCandle {
    pub at: String,
    pub open: HelperPriceValue,
    pub high: HelperPriceValue,
    pub low: HelperPriceValue,
    pub close: HelperPriceValue,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub volume: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperCandlesResponse {
    pub market: String,
    pub symbol: String,
    #[serde(alias = "instrument_id")]
    pub instrument_id: String,
    pub period: String,
    #[serde(default)]
    #[serde(alias = "extended_hours")]
    pub extended_hours: bool,
    #[serde(default, deserialize_with = "deserialize_vec_or_null")]
    pub candles: Vec<HelperCandle>,
    #[serde(alias = "total_returned")]
    pub total_returned: usize,
    #[serde(alias = "has_more")]
    pub has_more: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[serde(alias = "next_before")]
    pub next_before: Option<String>,
    pub source: String,
    #[serde(default)]
    pub adjustment: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn helper_price_value_validates_finite_numbers_and_rejects_invalid() {
        assert_eq!(
            serde_json::from_str::<HelperPriceValue>("123.45").unwrap(),
            HelperPriceValue("123.45".to_owned())
        );
        assert_eq!(
            serde_json::from_str::<HelperPriceValue>("\" 123.45 \"").unwrap(),
            HelperPriceValue("123.45".to_owned())
        );
        assert_eq!(
            serde_json::from_str::<HelperPriceValue>("100").unwrap(),
            HelperPriceValue("100".to_owned())
        );

        assert!(serde_json::from_str::<HelperPriceValue>("\"\"").is_err());
        assert!(serde_json::from_str::<HelperPriceValue>("\"   \"").is_err());
        assert!(serde_json::from_str::<HelperPriceValue>("\"abc\"").is_err());
        assert!(serde_json::from_str::<HelperPriceValue>("\"NaN\"").is_err());
        assert!(serde_json::from_str::<HelperPriceValue>("\"Infinity\"").is_err());
        assert!(serde_json::from_str::<HelperPriceValue>("\"-Infinity\"").is_err());
    }

    #[test]
    fn helper_dtos_accept_sidecar_snake_case_fields() {
        let markets: HelperMarketsResponse = serde_json::from_value(serde_json::json!({
            "markets": [{
                "code": "US",
                "resolved_market": "US",
                "preferred_prefix": "",
                "display_name": "United States",
                "quote_currency": "USD",
                "timezone": "America/New_York",
                "supports_extended_hours": true,
                "requires_exchange_prefix": false,
                "aliases": [],
                "regular_sessions": [{"start_minute": 570, "end_minute": 960, "label": "regular"}],
                "precision": {"price": 2, "quote": 2},
                "tick_size": 0.01
            }],
            "default_market": "US"
        }))
        .expect("snake_case markets response");
        assert_eq!(markets.markets[0].regular_sessions[0].start_minute, 570);

        let search: HelperSearchResponse = serde_json::from_value(serde_json::json!({
            "entries": [{
                "market": "US", "resolved_market": "US", "instrument_id": "US.AAPL",
                "code": "AAPL", "symbol": "AAPL", "selectable": true, "source": "yfinance",
                "supported_periods": ["1d"], "security_type": "stock"
            }]
        }))
        .expect("snake_case search response");
        assert_eq!(search.entries[0].instrument_id, "US.AAPL");

        let security: HelperSecurityResponse = serde_json::from_value(serde_json::json!({
            "market": "US", "symbol": "AAPL", "instrument_id": "US.AAPL", "name": "Apple",
            "security_type": "stock", "supported_periods": ["1d"], "source": "yfinance"
        }))
        .expect("snake_case security response");
        assert_eq!(security.instrument_id, "US.AAPL");

        let snapshot: HelperSnapshotResponse = serde_json::from_value(serde_json::json!({
            "market": "US", "symbol": "AAPL", "instrument_id": "US.AAPL", "price": 1,
            "open_price": 1, "previous_close_price": 1, "regular_quote": {
                "high_price": 2, "change_value": 0.1, "quote_at": "2026-01-01T00:00:00Z"
            }, "observed_at": "2026-01-01T00:00:00Z", "source": "yfinance",
            "delayed": false, "delay_minutes": 0
        }))
        .expect("snake_case snapshot response");
        assert_eq!(
            snapshot
                .regular_quote
                .unwrap()
                .change_value
                .unwrap()
                .as_str(),
            "0.1"
        );

        let candles: HelperCandlesResponse = serde_json::from_value(serde_json::json!({
            "market": "US", "symbol": "AAPL", "instrument_id": "US.AAPL", "period": "1d",
            "extended_hours": false, "candles": [{"at": "2026-01-01T00:00:00Z", "open": 1,
            "high": 2, "low": 1, "close": 2, "volume": 10}], "total_returned": 1,
            "has_more": false, "next_before": null, "source": "yfinance", "adjustment": "none"
        }))
        .expect("snake_case candles response");
        assert_eq!(candles.total_returned, 1);
    }
}
