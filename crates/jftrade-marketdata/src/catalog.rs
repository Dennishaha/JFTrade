use rust_decimal::Decimal;
use serde::{Deserialize, Serialize};

/// Regular trading session window (minutes since midnight).
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TradingSessionWindow {
    pub start_minute: u32,
    pub end_minute: u32,
    pub label: String,
}

/// Market price and quote precision specification.
#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MarketPrecision {
    pub price: u32,
    pub quote: u32,
}

/// SSOT Market Rule definition for JFTrade.
#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MarketRule {
    pub code: String,
    pub market: String,
    pub resolved_market: String,
    pub preferred_prefix: String,
    pub name: String,
    pub display_name: String,
    pub quote_currency: String,
    pub timezone: String,
    pub supports_extended_hours: bool,
    pub requires_exchange_prefix: bool,
    pub aliases: Vec<String>,
    pub regular_sessions: Vec<TradingSessionWindow>,
    pub precision: MarketPrecision,
    pub tick_size: Decimal,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parent_market: Option<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub child_markets: Vec<String>,
}

impl MarketRule {
    /// Serializes this market rule to the wire format required by `GET /api/v1/market-data/markets`.
    pub fn to_api_value(&self) -> serde_json::Value {
        serde_json::json!({
            "code": self.code,
            "market": self.market,
            "resolvedMarket": self.resolved_market,
            "preferredPrefix": self.preferred_prefix,
            "name": self.name,
            "displayName": self.display_name,
            "quoteCurrency": self.quote_currency,
            "timezone": self.timezone,
            "supportsExtendedHours": self.supports_extended_hours,
            "requiresExchangePrefix": self.requires_exchange_prefix,
            "aliases": self.aliases,
            "regularSessions": self.regular_sessions,
            "precision": {
                "price": self.precision.price,
                "quote": self.precision.quote,
            },
            "tickSize": self.tick_size,
        })
    }

    /// Aligns a given price to this market's tick size using Decimal arithmetic.
    pub fn align_price_to_step(&self, price: Decimal) -> Decimal {
        if self.tick_size.is_zero() {
            return price;
        }
        (price / self.tick_size).round() * self.tick_size
    }
}

pub fn us_market_rule() -> MarketRule {
    MarketRule {
        code: "US".to_owned(),
        market: "US".to_owned(),
        resolved_market: "US".to_owned(),
        preferred_prefix: "US".to_owned(),
        name: "United States".to_owned(),
        display_name: "United States".to_owned(),
        quote_currency: "USD".to_owned(),
        timezone: "America/New_York".to_owned(),
        supports_extended_hours: true,
        requires_exchange_prefix: false,
        aliases: vec![
            "NYSE".to_owned(),
            "NASDAQ".to_owned(),
            "AMEX".to_owned(),
            "USA".to_owned(),
        ],
        regular_sessions: vec![TradingSessionWindow {
            start_minute: 570,
            end_minute: 960,
            label: "09:30-16:00".to_owned(),
        }],
        precision: MarketPrecision { price: 2, quote: 2 },
        tick_size: Decimal::new(1, 2), // 0.01
        parent_market: None,
        child_markets: vec![],
    }
}

pub fn hk_market_rule() -> MarketRule {
    MarketRule {
        code: "HK".to_owned(),
        market: "HK".to_owned(),
        resolved_market: "HK".to_owned(),
        preferred_prefix: "HK".to_owned(),
        name: "Hong Kong".to_owned(),
        display_name: "Hong Kong".to_owned(),
        quote_currency: "HKD".to_owned(),
        timezone: "Asia/Hong_Kong".to_owned(),
        supports_extended_hours: false,
        requires_exchange_prefix: false,
        aliases: vec!["HKEX".to_owned(), "HKG".to_owned()],
        regular_sessions: vec![
            TradingSessionWindow {
                start_minute: 570,
                end_minute: 720,
                label: "09:30-12:00".to_owned(),
            },
            TradingSessionWindow {
                start_minute: 780,
                end_minute: 960,
                label: "13:00-16:00".to_owned(),
            },
        ],
        precision: MarketPrecision { price: 3, quote: 3 },
        tick_size: Decimal::new(1, 3), // 0.001
        parent_market: None,
        child_markets: vec![],
    }
}

pub fn cn_market_rule() -> MarketRule {
    MarketRule {
        code: "CN".to_owned(),
        market: "CN".to_owned(),
        resolved_market: "CN".to_owned(),
        preferred_prefix: String::new(),
        name: "沪深".to_owned(),
        display_name: "沪深".to_owned(),
        quote_currency: "CNY".to_owned(),
        timezone: "Asia/Shanghai".to_owned(),
        supports_extended_hours: false,
        requires_exchange_prefix: true,
        aliases: vec![
            "SH".to_owned(),
            "SZ".to_owned(),
            "CNSH".to_owned(),
            "CNSZ".to_owned(),
        ],
        regular_sessions: vec![
            TradingSessionWindow {
                start_minute: 570,
                end_minute: 690,
                label: "09:30-11:30".to_owned(),
            },
            TradingSessionWindow {
                start_minute: 780,
                end_minute: 900,
                label: "13:00-15:00".to_owned(),
            },
        ],
        precision: MarketPrecision { price: 2, quote: 2 },
        tick_size: Decimal::new(1, 2), // 0.01
        parent_market: None,
        child_markets: vec!["SH".to_owned(), "SZ".to_owned()],
    }
}

pub fn sh_market_rule() -> MarketRule {
    MarketRule {
        code: "SH".to_owned(),
        market: "SH".to_owned(),
        resolved_market: "CN".to_owned(),
        preferred_prefix: "SH".to_owned(),
        name: "Shanghai".to_owned(),
        display_name: "Shanghai".to_owned(),
        quote_currency: "CNY".to_owned(),
        timezone: "Asia/Shanghai".to_owned(),
        supports_extended_hours: false,
        requires_exchange_prefix: true,
        aliases: vec![
            "CNSH".to_owned(),
            "SSE".to_owned(),
            "SHH".to_owned(),
            "SHSE".to_owned(),
            "SS".to_owned(),
        ],
        regular_sessions: vec![
            TradingSessionWindow {
                start_minute: 570,
                end_minute: 690,
                label: "09:30-11:30".to_owned(),
            },
            TradingSessionWindow {
                start_minute: 780,
                end_minute: 900,
                label: "13:00-15:00".to_owned(),
            },
        ],
        precision: MarketPrecision { price: 2, quote: 2 },
        tick_size: Decimal::new(1, 2), // 0.01
        parent_market: Some("CN".to_owned()),
        child_markets: vec![],
    }
}

pub fn sz_market_rule() -> MarketRule {
    MarketRule {
        code: "SZ".to_owned(),
        market: "SZ".to_owned(),
        resolved_market: "CN".to_owned(),
        preferred_prefix: "SZ".to_owned(),
        name: "Shenzhen".to_owned(),
        display_name: "Shenzhen".to_owned(),
        quote_currency: "CNY".to_owned(),
        timezone: "Asia/Shanghai".to_owned(),
        supports_extended_hours: false,
        requires_exchange_prefix: true,
        aliases: vec![
            "CNSZ".to_owned(),
            "SZSE".to_owned(),
            "SHZ".to_owned(),
            "SHE".to_owned(),
        ],
        regular_sessions: vec![
            TradingSessionWindow {
                start_minute: 570,
                end_minute: 690,
                label: "09:30-11:30".to_owned(),
            },
            TradingSessionWindow {
                start_minute: 780,
                end_minute: 900,
                label: "13:00-15:00".to_owned(),
            },
        ],
        precision: MarketPrecision { price: 2, quote: 2 },
        tick_size: Decimal::new(1, 2), // 0.01
        parent_market: Some("CN".to_owned()),
        child_markets: vec![],
    }
}

/// Returns the SSOT list of default markets: HK, US, CN, SH, SZ.
pub fn default_markets() -> Vec<MarketRule> {
    vec![
        hk_market_rule(),
        us_market_rule(),
        cn_market_rule(),
        sh_market_rule(),
        sz_market_rule(),
    ]
}

/// Look up a market rule by code or alias.
pub fn find_market_rule(code_or_alias: &str) -> Option<MarketRule> {
    let normalized = code_or_alias.trim().to_ascii_uppercase();
    if normalized.is_empty() {
        return None;
    }
    let markets = default_markets();
    // 1. Exact match on code or market name
    if let Some(exact) = markets.iter().find(|rule| {
        rule.code.eq_ignore_ascii_case(&normalized) || rule.market.eq_ignore_ascii_case(&normalized)
    }) {
        return Some(exact.clone());
    }
    // 2. Leaf market rules (non-aggregates) matching preferred prefix or alias
    if let Some(leaf) = markets.iter().find(|rule| {
        rule.child_markets.is_empty()
            && (rule.preferred_prefix.eq_ignore_ascii_case(&normalized)
                || rule
                    .aliases
                    .iter()
                    .any(|a| a.eq_ignore_ascii_case(&normalized)))
    }) {
        return Some(leaf.clone());
    }
    // 3. Aggregate rules matching preferred prefix or alias
    markets.into_iter().find(|rule| {
        rule.preferred_prefix.eq_ignore_ascii_case(&normalized)
            || rule
                .aliases
                .iter()
                .any(|a| a.eq_ignore_ascii_case(&normalized))
    })
}

/// Normalization result according to Go/JFTrade semantics.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct NormalizedInstrument {
    pub code: String,
    pub instrument_id: String,
    pub market: String,
    pub prefix: String,
    pub resolved_market: String,
    pub symbol: String,
}

#[derive(Clone, Debug, PartialEq, thiserror::Error)]
pub enum MarketCatalogError {
    #[error("symbol or code is required")]
    MissingSymbolOrCode,
    #[error("unsupported market: {0}")]
    UnsupportedMarket(String),
    #[error("market {0} does not match symbol {1}")]
    MarketMismatch(String, String),
}

/// Returns true if the given code is a supported auxiliary market identifier.
fn is_auxiliary_market(market: &str) -> bool {
    matches!(
        market.trim().to_ascii_uppercase().as_str(),
        "SG" | "JP" | "AU" | "MY" | "CA"
    )
}

/// Returns true if the given market code or alias is supported by JFTrade.
pub fn is_supported_market(market: &str) -> bool {
    let upper = market.trim().to_ascii_uppercase();
    if upper.is_empty() {
        return false;
    }
    find_market_rule(&upper).is_some() || is_auxiliary_market(&upper)
}

/// Validates whether an explicitly provided market is compatible with an instrument's exchange prefix.
fn check_market_compatibility(explicit_market: &str, symbol_prefix: &str) -> bool {
    let m_upper = explicit_market.trim().to_ascii_uppercase();
    let p_upper = symbol_prefix.trim().to_ascii_uppercase();

    if m_upper == p_upper {
        return true;
    }

    let m_rule = find_market_rule(&m_upper);
    let p_rule = find_market_rule(&p_upper);

    match (m_rule, p_rule) {
        (Some(m), Some(p)) => {
            if m.code == p.code {
                return true;
            }
            if m.child_markets.contains(&p.code) {
                return true;
            }
            if m.code == "CN" && p.resolved_market == "CN" {
                return true;
            }
            false
        }
        _ => false,
    }
}

/// Infer the exchange prefix (SH or SZ) for a Chinese A-share code.
pub fn infer_cn_prefix(symbol: &str) -> &'static str {
    let clean = symbol.trim().to_ascii_uppercase();

    // 1. Prioritize explicit exchange prefixes or suffixes if already present
    if clean.starts_with("SH.")
        || clean.starts_with("SH:")
        || clean.starts_with("SH_")
        || clean.starts_with("CNSH.")
        || clean.starts_with("CNSH:")
        || clean.starts_with("CNSH_")
        || clean.ends_with(".SH")
        || clean.ends_with(":SH")
        || clean.ends_with("_SH")
        || clean.ends_with(".SS")
        || clean.ends_with(":SS")
    {
        return "SH";
    }
    if clean.starts_with("SZ.")
        || clean.starts_with("SZ:")
        || clean.starts_with("SZ_")
        || clean.starts_with("CNSZ.")
        || clean.starts_with("CNSZ:")
        || clean.starts_with("CNSZ_")
        || clean.ends_with(".SZ")
        || clean.ends_with(":SZ")
        || clean.ends_with("_SZ")
    {
        return "SZ";
    }

    // 2. Strip optional generic CN prefix or suffix if present
    let raw = clean
        .strip_prefix("CN.")
        .or_else(|| clean.strip_prefix("CN:"))
        .or_else(|| clean.strip_prefix("CN_"))
        .unwrap_or(&clean);

    let digits = raw
        .strip_suffix(".CN")
        .or_else(|| raw.strip_suffix(":CN"))
        .unwrap_or(raw);

    // 3. Infer prefix from Shanghai numeric code allocations:
    // - 6xxxxx: Shanghai Main Board & STAR Market (688/689)
    // - 9xxxxx: Shanghai B-shares
    // - 5xxxxx: Shanghai Funds / ETFs
    // - 11xxxx, 132xxx: Shanghai Convertible bonds
    // - 73xxxx: Shanghai IPO subscriptions
    // - 70xxxx, 71xxxx: Shanghai Rights offerings
    if digits.starts_with('6')
        || digits.starts_with('9')
        || digits.starts_with('5')
        || digits.starts_with("11")
        || digits.starts_with("132")
        || digits.starts_with("73")
        || digits.starts_with("70")
        || digits.starts_with("71")
    {
        "SH"
    } else {
        "SZ"
    }
}

/// Normalizes instrument identifiers across markets according to Go/JFTrade semantics.
pub fn normalize_instrument(
    market: Option<&str>,
    symbol: Option<&str>,
) -> Result<NormalizedInstrument, MarketCatalogError> {
    let raw_market = market.map(str::trim).filter(|m| !m.is_empty());
    let raw_symbol = symbol.map(str::trim).filter(|s| !s.is_empty());

    if let Some(m) = raw_market.filter(|m| !is_supported_market(m)) {
        return Err(MarketCatalogError::UnsupportedMarket(m.to_owned()));
    }

    let (m_part, s_part): (String, String) = match (raw_market, raw_symbol) {
        (Some(m), Some(s)) => {
            let s_clean = s.replace(':', ".");
            if let Some((first, second)) = s_clean.split_once('.') {
                let f = first.trim();
                let sec = second.trim();
                if f.is_empty() || sec.is_empty() {
                    return Err(MarketCatalogError::MissingSymbolOrCode);
                }
                let f_upper = f.to_ascii_uppercase();
                let sec_upper = sec.to_ascii_uppercase();

                // Determine whether format is prefix (EXCHANGE.CODE) or suffix (CODE.EXCHANGE)
                let (p, c) = if !is_supported_market(&f_upper) && is_supported_market(&sec_upper) {
                    (sec, f)
                } else {
                    (f, sec)
                };

                if !is_supported_market(p) {
                    return Err(MarketCatalogError::UnsupportedMarket(p.to_owned()));
                }

                if !check_market_compatibility(m, p) {
                    return Err(MarketCatalogError::MarketMismatch(
                        m.to_owned(),
                        s.to_owned(),
                    ));
                }
                (p.to_owned(), c.to_owned())
            } else {
                (m.to_owned(), s.to_owned())
            }
        }
        (None, Some(s)) => {
            let s_clean = s.replace(':', ".");
            if let Some((first, second)) = s_clean.split_once('.') {
                let f = first.trim();
                let sec = second.trim();
                if f.is_empty() || sec.is_empty() {
                    return Err(MarketCatalogError::MissingSymbolOrCode);
                }
                let f_upper = f.to_ascii_uppercase();
                let sec_upper = sec.to_ascii_uppercase();

                let (p, c) = if !is_supported_market(&f_upper) && is_supported_market(&sec_upper) {
                    (sec, f)
                } else {
                    (f, sec)
                };

                if !is_supported_market(p) {
                    return Err(MarketCatalogError::UnsupportedMarket(p.to_owned()));
                }
                (p.to_owned(), c.to_owned())
            } else {
                ("US".to_owned(), s.to_owned())
            }
        }
        (Some(_), None) => return Err(MarketCatalogError::MissingSymbolOrCode),
        (None, None) => return Err(MarketCatalogError::MissingSymbolOrCode),
    };

    if s_part.is_empty() {
        return Err(MarketCatalogError::MissingSymbolOrCode);
    }

    let m_upper = m_part.to_ascii_uppercase();
    let s_upper = s_part.to_ascii_uppercase();

    match m_upper.as_str() {
        "CN" => {
            let prefix = infer_cn_prefix(&s_upper);
            let instrument_id = format!("{prefix}.{s_upper}");
            Ok(NormalizedInstrument {
                code: s_upper.clone(),
                instrument_id: instrument_id.clone(),
                market: "CN".to_owned(),
                prefix: prefix.to_owned(),
                resolved_market: "CN".to_owned(),
                symbol: instrument_id,
            })
        }
        "SH" | "CNSH" | "SSE" | "SHH" | "SHSE" | "SS" => {
            let instrument_id = format!("SH.{s_upper}");
            Ok(NormalizedInstrument {
                code: s_upper.clone(),
                instrument_id: instrument_id.clone(),
                market: "CN".to_owned(),
                prefix: "SH".to_owned(),
                resolved_market: "CN".to_owned(),
                symbol: instrument_id,
            })
        }
        "SZ" | "CNSZ" | "SZSE" | "SHZ" | "SHE" => {
            let instrument_id = format!("SZ.{s_upper}");
            Ok(NormalizedInstrument {
                code: s_upper.clone(),
                instrument_id: instrument_id.clone(),
                market: "CN".to_owned(),
                prefix: "SZ".to_owned(),
                resolved_market: "CN".to_owned(),
                symbol: instrument_id,
            })
        }
        "HK" | "HKEX" | "HKG" => {
            let code = if s_upper.chars().all(|c| c.is_ascii_digit()) && s_upper.len() < 5 {
                format!("{s_upper:0>5}")
            } else {
                s_upper.clone()
            };
            let instrument_id = format!("HK.{code}");
            Ok(NormalizedInstrument {
                code,
                instrument_id: instrument_id.clone(),
                market: "HK".to_owned(),
                prefix: "HK".to_owned(),
                resolved_market: "HK".to_owned(),
                symbol: instrument_id,
            })
        }
        "US" | "NYSE" | "NASDAQ" | "AMEX" | "USA" => {
            let instrument_id = format!("US.{s_upper}");
            Ok(NormalizedInstrument {
                code: s_upper.clone(),
                instrument_id: instrument_id.clone(),
                market: "US".to_owned(),
                prefix: "US".to_owned(),
                resolved_market: "US".to_owned(),
                symbol: instrument_id,
            })
        }
        "SG" | "JP" | "AU" | "MY" | "CA" => {
            let instrument_id = format!("{m_upper}.{s_upper}");
            Ok(NormalizedInstrument {
                code: s_upper.clone(),
                instrument_id: instrument_id.clone(),
                market: m_upper.clone(),
                prefix: m_upper.clone(),
                resolved_market: m_upper,
                symbol: instrument_id,
            })
        }
        other => Err(MarketCatalogError::UnsupportedMarket(other.to_owned())),
    }
}

#[cfg(test)]
#[path = "catalog_tests.rs"]
mod tests;
