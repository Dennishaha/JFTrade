use serde::{Deserialize, Serialize};

use jftrade_kernel::WireTimestamp;

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum SessionState {
    Disconnected,
    Connecting,
    ReadOnlyReady,
    TradeReady,
    Reconnecting,
    Closing,
    Closed,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct AccountSnapshot {
    pub account_ids: Vec<String>,
    pub generation: u64,
    pub refreshed_at: WireTimestamp,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BrokerSession {
    state: SessionState,
    generation: u64,
    quote_login: bool,
    trade_login: bool,
    unlocked: bool,
    account_snapshot: Option<AccountSnapshot>,
}

impl Default for BrokerSession {
    fn default() -> Self {
        Self {
            state: SessionState::Disconnected,
            generation: 0,
            quote_login: false,
            trade_login: false,
            unlocked: false,
            account_snapshot: None,
        }
    }
}

impl BrokerSession {
    pub fn connect(&mut self) -> bool {
        if matches!(self.state, SessionState::Closing | SessionState::Closed) {
            return false;
        }
        self.state = SessionState::Connecting;
        true
    }

    pub fn authenticated(&mut self, quote_login: bool, trade_login: bool, unlocked: bool) -> bool {
        if self.state != SessionState::Connecting && self.state != SessionState::Reconnecting {
            return false;
        }
        self.quote_login = quote_login;
        self.trade_login = trade_login;
        self.unlocked = unlocked;
        self.state = if quote_login && trade_login && unlocked {
            SessionState::TradeReady
        } else if quote_login {
            SessionState::ReadOnlyReady
        } else {
            SessionState::Disconnected
        };
        true
    }

    pub fn disconnected(&mut self) -> bool {
        if matches!(self.state, SessionState::Closing | SessionState::Closed) {
            return false;
        }
        self.generation += 1;
        self.quote_login = false;
        self.trade_login = false;
        self.unlocked = false;
        self.account_snapshot = None;
        self.state = SessionState::Reconnecting;
        true
    }

    pub fn refresh_accounts(&mut self, snapshot: AccountSnapshot) -> bool {
        if snapshot.generation != self.generation
            || !matches!(
                self.state,
                SessionState::ReadOnlyReady | SessionState::TradeReady
            )
        {
            return false;
        }
        self.account_snapshot = Some(snapshot);
        true
    }

    pub fn begin_close(&mut self) -> bool {
        if matches!(self.state, SessionState::Closing | SessionState::Closed) {
            return false;
        }
        self.state = SessionState::Closing;
        true
    }

    pub fn finish_close(&mut self) -> bool {
        if self.state == SessionState::Closed {
            return false;
        }
        self.state = SessionState::Closed;
        self.account_snapshot = None;
        true
    }

    pub const fn state(&self) -> SessionState {
        self.state
    }

    pub const fn generation(&self) -> u64 {
        self.generation
    }

    pub const fn can_read(&self) -> bool {
        matches!(
            self.state,
            SessionState::ReadOnlyReady | SessionState::TradeReady
        )
    }

    pub const fn can_trade(&self) -> bool {
        matches!(self.state, SessionState::TradeReady)
    }
}

#[cfg(test)]
mod tests {
    use super::{AccountSnapshot, BrokerSession, SessionState};

    #[test]
    fn reconnect_invalidates_accounts_and_rejects_stale_refresh() {
        let mut session = BrokerSession::default();
        assert!(session.connect());
        assert!(session.authenticated(true, true, true));
        assert!(session.refresh_accounts(AccountSnapshot {
            account_ids: vec!["acc-1".to_owned()],
            generation: 0,
            refreshed_at: "2026-08-19T00:00:00Z".parse().expect("timestamp"),
        }));
        assert!(session.disconnected());
        assert_eq!(session.state(), SessionState::Reconnecting);
        assert!(!session.refresh_accounts(AccountSnapshot {
            account_ids: vec!["stale".to_owned()],
            generation: 0,
            refreshed_at: "2026-08-19T00:00:01Z".parse().expect("timestamp"),
        }));
        assert!(session.authenticated(true, false, false));
        assert!(session.can_read());
        assert!(!session.can_trade());
    }

    #[test]
    fn close_is_bounded_by_idempotent_state_transitions() {
        let mut session = BrokerSession::default();
        assert!(session.begin_close());
        assert!(!session.begin_close());
        assert!(session.finish_close());
        assert!(!session.finish_close());
        assert!(!session.connect());
    }
}
