use std::collections::{BTreeMap, BTreeSet};
use std::sync::Arc;

use crate::{
    CalendarManagerError, CalendarSourceDescriptor, CalendarSourcePolicy, CalendarSourcePort,
};

#[derive(Default)]
pub struct CalendarSourceRegistry {
    sources: BTreeMap<String, Arc<dyn CalendarSourcePort>>,
    order: Vec<String>,
}

impl CalendarSourceRegistry {
    pub fn register(
        &mut self,
        source: Arc<dyn CalendarSourcePort>,
    ) -> Result<(), CalendarManagerError> {
        let descriptor = normalized_descriptor(source.descriptor());
        if descriptor.id.is_empty() {
            return Err(CalendarManagerError::InvalidSettings(
                "calendar source id is required".to_owned(),
            ));
        }
        if !self.sources.contains_key(&descriptor.id) {
            self.order.push(descriptor.id.clone());
        }
        self.sources.insert(descriptor.id, source);
        Ok(())
    }

    pub fn source(&self, source_id: &str) -> Option<Arc<dyn CalendarSourcePort>> {
        self.sources.get(source_id.trim()).cloned()
    }

    pub fn descriptors(&self) -> Vec<CalendarSourceDescriptor> {
        self.sources
            .values()
            .map(|source| normalized_descriptor(source.descriptor()))
            .collect()
    }

    pub(crate) fn ordered_sources(
        &self,
        market: &str,
        policy: &CalendarSourcePolicy,
    ) -> Vec<Arc<dyn CalendarSourcePort>> {
        let enabled = policy
            .enabled_source_ids
            .iter()
            .map(|id| id.trim())
            .filter(|id| !id.is_empty())
            .collect::<BTreeSet<_>>();
        let mut ids = policy
            .preferred_source_ids
            .iter()
            .map(|id| id.trim().to_owned())
            .collect::<Vec<_>>();
        ids.extend(self.order.iter().cloned());
        let mut seen = BTreeSet::new();
        ids.into_iter()
            .filter(|id| enabled.is_empty() || enabled.contains(id.as_str()))
            .filter(|id| seen.insert(id.clone()))
            .filter_map(|id| self.source(&id))
            .filter(|source| source_supports(&source.descriptor(), market))
            .collect()
    }

    pub(crate) fn lifecycle_sources(&self) -> Vec<Arc<dyn CalendarSourcePort>> {
        self.order.iter().filter_map(|id| self.source(id)).collect()
    }
}

fn normalized_descriptor(mut descriptor: CalendarSourceDescriptor) -> CalendarSourceDescriptor {
    descriptor.id = descriptor.id.trim().to_owned();
    descriptor.kind = descriptor.kind.trim().to_owned();
    descriptor.authority = descriptor.authority.trim().to_owned();
    descriptor.markets = descriptor
        .markets
        .into_iter()
        .map(|market| market.trim().to_uppercase())
        .filter(|market| !market.is_empty())
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect();
    descriptor
}

fn source_supports(descriptor: &CalendarSourceDescriptor, market: &str) -> bool {
    descriptor.markets.iter().any(|candidate| {
        let candidate = candidate.trim().to_uppercase();
        candidate == market || (candidate == "CN" && matches!(market, "SH" | "SZ"))
    })
}
