use std::fmt;

use serde::de;
use serde::{Deserialize, Deserializer, Serialize, Serializer};

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
    pub start_minute: i32,
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
    pub resolved_market: String,
    pub preferred_prefix: String,
    pub display_name: String,
    pub quote_currency: String,
    pub timezone: String,
    pub supports_extended_hours: bool,
    pub requires_exchange_prefix: bool,
    #[serde(default)]
    pub aliases: Vec<String>,
    #[serde(default)]
    pub regular_sessions: Vec<HelperTradingWindow>,
    pub precision: HelperMarketPrecision,
    pub tick_size: f64,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperMarketsResponse {
    pub markets: Vec<HelperMarketProfile>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub default_market: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSearchEntry {
    pub market: String,
    pub resolved_market: String,
    pub instrument_id: String,
    pub code: String,
    pub symbol: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub security_type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exchange: Option<String>,
    pub selectable: bool,
    pub source: String,
    #[serde(default)]
    pub supported_periods: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSearchResponse {
    pub entries: Vec<HelperSearchEntry>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSecurityResponse {
    pub market: String,
    pub symbol: String,
    pub instrument_id: String,
    pub name: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exchange: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub currency: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub timezone: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub security_type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub industry: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sector: Option<String>,
    #[serde(default)]
    pub supported_periods: Vec<String>,
    pub source: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSnapshotQuote {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub high_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub low_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub volume: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub turnover: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub change_value: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub change_rate: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub quote_at: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperSnapshotResponse {
    pub market: String,
    pub symbol: String,
    pub instrument_id: String,
    pub price: HelperPriceValue,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bid: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ask: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub open_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub high_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub low_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub previous_close_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_close_price: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub regular_quote: Option<HelperSnapshotQuote>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub pre_market_quote: Option<HelperSnapshotQuote>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub after_market_quote: Option<HelperSnapshotQuote>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub volume: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub turnover: Option<HelperPriceValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub quote_at: Option<String>,
    pub observed_at: String,
    pub source: String,
    #[serde(default)]
    pub delayed: bool,
    #[serde(default)]
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
    pub instrument_id: String,
    pub period: String,
    #[serde(default)]
    pub extended_hours: bool,
    pub candles: Vec<HelperCandle>,
    pub total_returned: usize,
    pub has_more: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
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
}
