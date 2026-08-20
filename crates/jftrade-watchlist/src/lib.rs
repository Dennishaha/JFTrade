#![forbid(unsafe_code)]

use std::collections::BTreeSet;

use serde::{Deserialize, Serialize};
use thiserror::Error;

pub const DEFAULT_GROUP_NAME: &str = "自选股";
pub const DEFAULT_PAGE_LIMIT: usize = 100;
pub const MAX_PAGE_LIMIT: usize = 500;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GroupRef {
    pub group_id: String,
    pub name: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Memberships {
    pub instrument_id: String,
    pub revision: i64,
    pub groups: Vec<GroupRef>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MembershipPlan {
    pub instrument_id: String,
    pub group_ids: Vec<String>,
    pub new_group_names: Vec<String>,
    pub expected_revision: u64,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum WatchlistError {
    #[error("instrumentId must use MARKET.SYMBOL")]
    InvalidInstrument,
    #[error("group name is required")]
    MissingGroupName,
    #[error("group name must not exceed 80 characters")]
    GroupNameTooLong,
}

pub fn group_name_key(value: &str) -> String {
    value.trim().to_lowercase()
}

pub fn normalize_instrument_id(value: &str) -> Result<String, WatchlistError> {
    let normalized = value.trim().to_uppercase();
    let Some((market, symbol)) = normalized.split_once('.') else {
        return Err(WatchlistError::InvalidInstrument);
    };
    let canonical_market = match market {
        "US" | "HK" | "SH" | "SZ" => market,
        "CNSH" => "SH",
        "CNSZ" => "SZ",
        _ => return Err(WatchlistError::InvalidInstrument),
    };
    if market.is_empty()
        || symbol.is_empty()
        || symbol.contains('.')
        || !symbol
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_'))
    {
        return Err(WatchlistError::InvalidInstrument);
    }
    Ok(format!("{canonical_market}.{symbol}"))
}

pub fn plan_membership_replace(
    instrument_id: &str,
    group_ids: impl IntoIterator<Item = String>,
    new_group_names: impl IntoIterator<Item = String>,
    expected_revision: u64,
) -> Result<MembershipPlan, WatchlistError> {
    let group_ids = group_ids
        .into_iter()
        .filter_map(|value| {
            let normalized = value.trim().to_owned();
            (!normalized.is_empty()).then_some(normalized)
        })
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect();
    let mut name_keys = BTreeSet::new();
    let mut normalized_names = Vec::new();
    for name in new_group_names {
        let normalized = normalize_group_name(&name)?;
        if name_keys.insert(group_name_key(&normalized)) {
            normalized_names.push(normalized);
        }
    }
    Ok(MembershipPlan {
        instrument_id: normalize_instrument_id(instrument_id)?,
        group_ids,
        new_group_names: normalized_names,
        expected_revision,
    })
}

pub fn normalize_limit(limit: usize) -> usize {
    if limit == 0 {
        DEFAULT_PAGE_LIMIT
    } else {
        limit.min(MAX_PAGE_LIMIT)
    }
}

fn normalize_group_name(value: &str) -> Result<String, WatchlistError> {
    let normalized = value.trim();
    if normalized.is_empty() {
        return Err(WatchlistError::MissingGroupName);
    }
    if normalized.chars().count() > 80 {
        return Err(WatchlistError::GroupNameTooLong);
    }
    Ok(normalized.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn membership_plan_normalizes_identity_and_deduplicates_inputs() {
        let plan = plan_membership_replace(
            " hk.00700 ",
            [" g2 ".into(), "g1".into(), "g1".into(), "".into()],
            [" Tech ".into(), "tech".into(), " 长线 ".into()],
            3,
        )
        .expect("valid plan");
        assert_eq!(plan.instrument_id, "HK.00700");
        assert_eq!(plan.group_ids, ["g1", "g2"]);
        assert_eq!(plan.new_group_names, ["Tech", "长线"]);
        assert_eq!(plan.expected_revision, 3);
    }

    #[test]
    fn malformed_identity_and_unbounded_pages_fail_closed() {
        assert_eq!(
            normalize_instrument_id("00700"),
            Err(WatchlistError::InvalidInstrument)
        );
        assert_eq!(normalize_limit(0), DEFAULT_PAGE_LIMIT);
        assert_eq!(normalize_limit(999), MAX_PAGE_LIMIT);
    }

    #[test]
    fn instrument_normalization_matches_supported_go_market_aliases() {
        assert_eq!(normalize_instrument_id(" us.aapl ").unwrap(), "US.AAPL");
        assert_eq!(normalize_instrument_id("CNSH.600519").unwrap(), "SH.600519");
        assert_eq!(
            normalize_instrument_id("BAD.AAPL"),
            Err(WatchlistError::InvalidInstrument)
        );
    }
}
