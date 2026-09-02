use std::collections::BTreeSet;
use std::fmt;

use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Eq, Ord, PartialEq, PartialOrd, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RouteSpec {
    pub method: String,
    pub path: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RouteCatalog {
    routes: Vec<RouteSpec>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum RouteCatalogError {
    InvalidMethod(String),
    InvalidPath(String),
    Duplicate(String),
}

impl fmt::Display for RouteCatalogError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidMethod(method) => write!(formatter, "invalid HTTP method {method}"),
            Self::InvalidPath(path) => write!(formatter, "invalid API path {path}"),
            Self::Duplicate(route) => write!(formatter, "duplicate API route {route}"),
        }
    }
}

impl std::error::Error for RouteCatalogError {}

impl RouteCatalog {
    pub fn new(routes: impl IntoIterator<Item = RouteSpec>) -> Result<Self, RouteCatalogError> {
        let mut unique = BTreeSet::new();
        let mut normalized = Vec::new();
        for route in routes {
            let method = route.method.trim().to_uppercase();
            if !matches!(method.as_str(), "GET" | "POST" | "PUT" | "PATCH" | "DELETE") {
                return Err(RouteCatalogError::InvalidMethod(route.method));
            }
            let path = route.path.trim().to_owned();
            if !path.starts_with("/api/v1/") || path.contains('?') || path.ends_with('/') {
                return Err(RouteCatalogError::InvalidPath(path));
            }
            let key = format!("{method} {path}");
            if !unique.insert(key.clone()) {
                return Err(RouteCatalogError::Duplicate(key));
            }
            normalized.push(RouteSpec { method, path });
        }
        normalized.sort();
        Ok(Self { routes: normalized })
    }

    pub fn allows(&self, method: &str, concrete_path: &str) -> bool {
        let method = method.trim().to_uppercase();
        self.routes
            .iter()
            .any(|route| route.method == method && template_matches(&route.path, concrete_path))
    }

    pub fn routes(&self) -> &[RouteSpec] {
        &self.routes
    }
}

fn template_matches(template: &str, concrete: &str) -> bool {
    let template = template.split('/').collect::<Vec<_>>();
    let concrete = concrete.split('/').collect::<Vec<_>>();
    template.len() == concrete.len()
        && template.iter().zip(concrete).all(|(expected, actual)| {
            if expected.starts_with('{') && expected.ends_with('}') {
                !actual.is_empty()
            } else {
                *expected == actual
            }
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exact_method_and_single_segment_parameters_are_required() {
        let catalog = RouteCatalog::new([RouteSpec {
            method: "get".into(),
            path: "/api/v1/watchlist/groups/{groupId}".into(),
        }])
        .expect("catalog");
        assert!(catalog.allows("GET", "/api/v1/watchlist/groups/g1"));
        assert!(!catalog.allows("POST", "/api/v1/watchlist/groups/g1"));
        assert!(!catalog.allows("GET", "/api/v1/watchlist/groups/g1/more"));
        assert!(!catalog.allows("GET", "/api/v1/watchlist/groups/"));
    }
}
