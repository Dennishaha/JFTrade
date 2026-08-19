use std::collections::BTreeSet;

use serde::Serialize;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct NotificationEnvelope {
    pub source_event_id: String,
    pub trace_id: String,
    pub category: String,
    pub title: String,
    pub message: String,
    pub dispatch: bool,
    pub duplicate: bool,
}

#[derive(Clone, Debug, Default)]
pub struct NotificationPlanner {
    seen: BTreeSet<String>,
}

impl NotificationPlanner {
    pub fn plan(
        &mut self,
        source_event_id: &str,
        trace_id: &str,
        category: &str,
        title: &str,
        message: &str,
    ) -> Option<NotificationEnvelope> {
        if source_event_id.trim().is_empty()
            || trace_id.trim().is_empty()
            || category.trim().is_empty()
        {
            return None;
        }
        let duplicate = !self.seen.insert(source_event_id.to_owned());
        Some(NotificationEnvelope {
            source_event_id: source_event_id.to_owned(),
            trace_id: trace_id.to_owned(),
            category: category.to_owned(),
            title: title.to_owned(),
            message: message.to_owned(),
            dispatch: false,
            duplicate,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::NotificationPlanner;

    #[test]
    fn shadow_notifications_are_deduplicated_and_never_dispatched() {
        let mut planner = NotificationPlanner::default();
        let first = planner
            .plan("event-1", "trace-1", "order", "Order", "accepted")
            .expect("first");
        let replay = planner
            .plan("event-1", "trace-1", "order", "Order", "accepted")
            .expect("replay");
        assert!(!first.dispatch && !first.duplicate);
        assert!(!replay.dispatch && replay.duplicate);
    }
}
