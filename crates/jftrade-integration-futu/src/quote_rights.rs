//! Generation-fenced OpenD quote-rights state.
//!
//! Go `pkg/futu/adapter_capabilities.go` keeps one owner for connect status,
//! the latest quote-rights snapshot, the revision counter and the retryable
//! failure cache. Notifications are fenced by the active OpenD connection
//! generation so a stale `ConnStatus`/`QotRight` push cannot replace a newer
//! session's entitlement, and a late `GetUserInfo` response cannot overwrite a
//! notification that already resolved the same generation.
//!
//! Rust previously projected `QUOTE_RIGHT_AVAILABLE` purely from
//! `connection_ready`, which could report entitlements that OpenD never
//! confirmed and could not fail closed on a connection change. This module is
//! the single owner for that state; callers only observe it.

use std::time::Duration;

use crate::trade_proto::notify::QotRight;

/// Provider-neutral quote-right snapshot. Generated OpenD messages stay inside
/// this crate; the engine only ever sees this value.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct QuoteRightSnapshot {
    pub hk_qot_right: i32,
    pub us_qot_right: i32,
    pub cn_qot_right: i32,
    pub hk_option_qot_right: Option<i32>,
    pub has_us_option_qot_right: bool,
    pub us_option_qot_right: Option<i32>,
    pub us_index_qot_right: Option<i32>,
    pub hk_future_qot_right: Option<i32>,
    pub us_future_qot_right: Option<i32>,
    pub us_cme_future_qot_right: Option<i32>,
    pub us_cbot_future_qot_right: Option<i32>,
    pub us_nymex_future_qot_right: Option<i32>,
    pub us_comex_future_qot_right: Option<i32>,
    pub us_cboe_future_qot_right: Option<i32>,
    pub sh_qot_right: Option<i32>,
    pub sz_qot_right: Option<i32>,
    pub ec_qot_right: Option<i32>,
}

impl QuoteRightSnapshot {
    /// Go `quoteRightsFromUserInfo`: convert the GetUserInfo legacy shape into
    /// the QotRight snapshot without inferring detailed entitlements. A field
    /// that OpenD did not report stays `None`, so a legacy-only HK/CN field can
    /// never authorize SH/SZ or the US option product.
    pub fn from_user_info(info: &crate::trade_proto::get_user_info::S2c) -> Option<Self> {
        fn has_any(info: &crate::trade_proto::get_user_info::S2c) -> bool {
            info.hk_qot_right.is_some()
                || info.us_qot_right.is_some()
                || info.cn_qot_right.is_some()
                || info.sh_qot_right.is_some()
                || info.sz_qot_right.is_some()
                || info.hk_option_qot_right.is_some()
                || info.has_us_option_qot_right.is_some()
                || info.us_option_qot_right.is_some()
                || info.hk_future_qot_right.is_some()
                || info.us_future_qot_right.is_some()
                || info.us_index_qot_right.is_some()
                || info.us_otc_qot_right.is_some()
                || info.us_cme_future_qot_right.is_some()
                || info.us_cbot_future_qot_right.is_some()
                || info.us_nymex_future_qot_right.is_some()
                || info.us_comex_future_qot_right.is_some()
                || info.us_cboe_future_qot_right.is_some()
                || info.sg_future_qot_right.is_some()
                || info.jp_future_qot_right.is_some()
                || info.cc_qot_right.is_some()
                || info.sg_stock_qot_right.is_some()
                || info.my_stock_qot_right.is_some()
                || info.jp_stock_qot_right.is_some()
                || info.ec_qot_right.is_some()
        }
        if !has_any(info) {
            return None;
        }
        Some(Self {
            hk_qot_right: info.hk_qot_right.unwrap_or(0),
            us_qot_right: info.us_qot_right.unwrap_or(0),
            cn_qot_right: info.cn_qot_right.unwrap_or(0),
            hk_option_qot_right: info.hk_option_qot_right,
            has_us_option_qot_right: info.has_us_option_qot_right.unwrap_or(false),
            us_option_qot_right: info.us_option_qot_right,
            us_index_qot_right: info.us_index_qot_right,
            hk_future_qot_right: info.hk_future_qot_right,
            us_future_qot_right: info.us_future_qot_right,
            us_cme_future_qot_right: info.us_cme_future_qot_right,
            us_cbot_future_qot_right: info.us_cbot_future_qot_right,
            us_nymex_future_qot_right: info.us_nymex_future_qot_right,
            us_comex_future_qot_right: info.us_comex_future_qot_right,
            us_cboe_future_qot_right: info.us_cboe_future_qot_right,
            sh_qot_right: info.sh_qot_right,
            sz_qot_right: info.sz_qot_right,
            ec_qot_right: info.ec_qot_right,
        })
    }

    /// Test/composition helper; OpenD wire decoding is the only other writer.
    pub fn from_notification(rights: &QotRight) -> Self {
        Self {
            hk_qot_right: rights.hk_qot_right,
            us_qot_right: rights.us_qot_right,
            cn_qot_right: rights.cn_qot_right,
            hk_option_qot_right: rights.hk_option_qot_right,
            has_us_option_qot_right: rights.has_us_option_qot_right.unwrap_or(false),
            us_option_qot_right: rights.us_option_qot_right,
            us_index_qot_right: rights.us_index_qot_right,
            hk_future_qot_right: rights.hk_future_qot_right,
            us_future_qot_right: rights.us_future_qot_right,
            us_cme_future_qot_right: rights.us_cme_future_qot_right,
            us_cbot_future_qot_right: rights.us_cbot_future_qot_right,
            us_nymex_future_qot_right: rights.us_nymex_future_qot_right,
            us_comex_future_qot_right: rights.us_comex_future_qot_right,
            us_cboe_future_qot_right: rights.us_cboe_future_qot_right,
            sh_qot_right: rights.sh_qot_right,
            sz_qot_right: rights.sz_qot_right,
            ec_qot_right: rights.ec_qot_right,
        }
    }
}

pub const QUOTE_RIGHTS_FAILURE_RETRY_INTERVAL: Duration = Duration::from_secs(3);

/// State reported by the runtime capability projection.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum QuoteRightState {
    /// No verified entitlement for the active generation.
    Unverified,
    /// Verified Level-1/2/SF/Level-3 entitlement.
    Available,
    /// BMP access (polling only, streaming denied).
    PollingOnly,
    /// Explicitly denied for the requested product.
    Denied,
    /// Verified, but the reported value is outside the known table.
    Unknown,
}

/// Market/product selector for the Go `quoteRightForCapability` switch.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum QuoteRightField {
    Hk,
    Us,
    Sh,
    Sz,
    HkOption,
    UsOption,
    HkFuture,
    UsFuture,
    UsIndex,
    EventContract,
}

/// Go `maximumQuoteRight`: skip `No` entitlements unless every field is `No`.
fn maximum_quote_right(values: [i32; 6]) -> i32 {
    let mut maximum = 0;
    for value in values {
        if value != 5 && value > maximum {
            maximum = value;
        }
    }
    if maximum == 0 && values.contains(&5) {
        return 5;
    }
    maximum
}

impl QuoteRightState {
    pub fn from_right(value: i32) -> Self {
        match value {
            2 | 3 | 4 | 6 => Self::Available,
            1 => Self::PollingOnly,
            5 => Self::Denied,
            _ => Self::Unknown,
        }
    }

    pub fn code(self) -> &'static str {
        match self {
            Self::Unverified => "QUOTE_RIGHT_UNVERIFIED",
            Self::Available => "QUOTE_RIGHT_AVAILABLE",
            Self::PollingOnly => "QUOTE_RIGHT_POLLING_ONLY",
            Self::Denied => "QUOTE_RIGHT_DENIED",
            Self::Unknown => "QUOTE_RIGHT_UNKNOWN",
        }
    }
}

#[derive(Clone, Debug)]
pub struct ConnectStatusSnapshot {
    pub quote_logged_in: bool,
    pub trade_logged_in: bool,
    pub observed_at: std::time::SystemTime,
    pub generation: u64,
}

#[derive(Clone, Debug)]
struct QuoteRightsSnapshot {
    value: QuoteRightSnapshot,
    observed_at: std::time::SystemTime,
    generation: u64,
}

#[derive(Clone, Debug)]
struct QuoteRightsFailure {
    reason: String,
    retry_at: std::time::SystemTime,
    generation: u64,
}

/// The one owner of connect status and quote-rights state.
#[derive(Clone, Debug, Default)]
pub struct QuoteRightsState {
    connect_status: Option<ConnectStatusSnapshot>,
    rights: Option<QuoteRightsSnapshot>,
    failure: Option<QuoteRightsFailure>,
    revision: u64,
}

impl QuoteRightsState {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn revision(&self) -> u64 {
        self.revision
    }

    /// Verified entitlement snapshot for the active generation, if any.
    pub fn verified_rights(
        &self,
        generation: u64,
    ) -> Option<(&QuoteRightSnapshot, std::time::SystemTime)> {
        let snapshot = self.rights.as_ref()?;
        (generation != 0 && snapshot.generation == generation)
            .then_some((&snapshot.value, snapshot.observed_at))
    }

    pub fn connect_status_for_generation(&self, generation: u64) -> Option<&ConnectStatusSnapshot> {
        self.connect_status
            .as_ref()
            .filter(|status| status.generation == generation)
    }

    /// Go `captureCapabilityNotificationAt` for a `ConnStatus` push.
    pub fn store_connect_status(
        &mut self,
        generation: u64,
        quote_logged_in: bool,
        trade_logged_in: bool,
        observed_at: std::time::SystemTime,
    ) {
        if self
            .connect_status
            .as_ref()
            .is_some_and(|status| status.generation > generation)
        {
            return;
        }
        self.connect_status = Some(ConnectStatusSnapshot {
            quote_logged_in,
            trade_logged_in,
            observed_at,
            generation,
        });
    }

    /// Go `captureCapabilityNotificationAt` for a `QotRight` push. A stale
    /// generation is ignored; otherwise the push wins over an in-flight query
    /// for the same generation and clears only that generation's failure.
    pub fn store_quote_right(
        &mut self,
        generation: u64,
        rights: QuoteRightSnapshot,
        observed_at: std::time::SystemTime,
    ) -> bool {
        if self
            .rights
            .as_ref()
            .is_some_and(|snapshot| snapshot.generation > generation)
        {
            return false;
        }
        self.rights = Some(QuoteRightsSnapshot {
            value: rights,
            observed_at,
            generation,
        });
        if self
            .failure
            .as_ref()
            .is_some_and(|failure| failure.generation == generation)
        {
            self.failure = None;
        }
        self.revision += 1;
        true
    }

    /// Go `storeQuoteRights`: a notification that landed after the query
    /// started wins. Returns `true` when the query result was accepted,
    /// `false` when a newer notification already resolved this generation.
    pub fn store_query_result(
        &mut self,
        generation: u64,
        start_revision: u64,
        rights: QuoteRightSnapshot,
        observed_at: std::time::SystemTime,
    ) -> bool {
        let notification_won = self.revision != start_revision
            && self
                .rights
                .as_ref()
                .is_some_and(|snapshot| snapshot.generation == generation);
        if notification_won {
            if self
                .failure
                .as_ref()
                .is_some_and(|failure| failure.generation == generation)
            {
                self.failure = None;
            }
            return false;
        }
        if self
            .rights
            .as_ref()
            .is_some_and(|snapshot| snapshot.generation > generation)
        {
            return false;
        }
        self.rights = Some(QuoteRightsSnapshot {
            value: rights,
            observed_at,
            generation,
        });
        if self
            .failure
            .as_ref()
            .is_some_and(|failure| failure.generation == generation)
        {
            self.failure = None;
        }
        true
    }

    pub fn cached_failure_due(&self, generation: u64, now: std::time::SystemTime) -> Option<&str> {
        self.failure
            .as_ref()
            .filter(|failure| failure.generation == generation && now < failure.retry_at)
            .map(|failure| failure.reason.as_str())
    }

    pub fn remember_failure(
        &mut self,
        generation: u64,
        reason: impl Into<String>,
        now: std::time::SystemTime,
    ) {
        self.failure = Some(QuoteRightsFailure {
            reason: reason.into(),
            retry_at: now + QUOTE_RIGHTS_FAILURE_RETRY_INTERVAL,
            generation,
        });
    }

    /// Go `handleQuoteRightsFetchFailure`. When a same-generation
    /// notification has already resolved the entitlement the failure is
    /// swallowed; a generation mismatch asks the caller to refresh.
    pub fn handle_fetch_failure(
        &mut self,
        generation: u64,
        active_generation: u64,
        reason: impl Into<String>,
        now: std::time::SystemTime,
    ) -> QuoteRightsFetchOutcome {
        let reason = reason.into();
        if self
            .rights
            .as_ref()
            .is_some_and(|snapshot| snapshot.generation == generation)
        {
            return QuoteRightsFetchOutcome::ResolvedByNotification;
        }
        if active_generation != generation {
            return QuoteRightsFetchOutcome::RefreshRequired;
        }
        self.remember_failure(generation, reason.clone(), now);
        QuoteRightsFetchOutcome::Failed(reason)
    }

    /// Right value for one market/product field, fenced to the active
    /// generation. `None` means "unverified", never "allowed".
    pub fn right_value(&self, generation: u64, field: QuoteRightField) -> Option<i32> {
        let snapshot = self.rights.as_ref()?;
        if generation == 0 || snapshot.generation != generation {
            return None;
        }
        let rights = &snapshot.value;
        Some(match field {
            QuoteRightField::Hk => rights.hk_qot_right,
            QuoteRightField::Us => rights.us_qot_right,
            QuoteRightField::Sh => rights.sh_qot_right.unwrap_or(rights.cn_qot_right),
            QuoteRightField::Sz => rights.sz_qot_right.unwrap_or(rights.cn_qot_right),
            QuoteRightField::HkOption => rights.hk_option_qot_right.unwrap_or(0),
            QuoteRightField::UsOption => rights
                .us_option_qot_right
                .unwrap_or(if rights.has_us_option_qot_right { 2 } else { 0 }),
            QuoteRightField::HkFuture => rights.hk_future_qot_right.unwrap_or(0),
            QuoteRightField::UsFuture => maximum_quote_right([
                rights.us_future_qot_right.unwrap_or(0),
                rights.us_cme_future_qot_right.unwrap_or(0),
                rights.us_cbot_future_qot_right.unwrap_or(0),
                rights.us_nymex_future_qot_right.unwrap_or(0),
                rights.us_comex_future_qot_right.unwrap_or(0),
                rights.us_cboe_future_qot_right.unwrap_or(0),
            ]),
            QuoteRightField::UsIndex => rights.us_index_qot_right.unwrap_or(0),
            QuoteRightField::EventContract => rights.ec_qot_right.unwrap_or(0),
        })
    }

    /// Convenience wrapper used by tests and single-value projections.
    pub fn state_for_generation(&self, generation: u64, right_for_product: i32) -> QuoteRightState {
        if self.right_value(generation, QuoteRightField::Us).is_none() {
            return QuoteRightState::Unverified;
        }
        QuoteRightState::from_right(right_for_product)
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum QuoteRightsFetchOutcome {
    ResolvedByNotification,
    RefreshRequired,
    Failed(String),
}

#[derive(Debug, thiserror::Error, Eq, PartialEq)]
pub enum QuoteRightsError {
    #[error("quote rights refresh required")]
    RefreshRequired,
    #[error("OpenD exchange is not configured")]
    NotConfigured,
    #[error("{0}")]
    Failed(String),
}

impl QuoteRightsError {
    pub fn is_refresh_required(&self) -> bool {
        matches!(self, Self::RefreshRequired)
    }
}

#[cfg(test)]
#[path = "quote_rights_tests.rs"]
mod tests;
