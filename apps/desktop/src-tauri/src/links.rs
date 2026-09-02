use serde::Serialize;
use thiserror::Error;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(tag = "kind", content = "url", rename_all = "lowercase")]
pub enum LinkTarget {
    Docs(String),
    External(String),
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum LinkError {
    #[error("link is empty")]
    Empty,
    #[error("link contains unsafe characters")]
    UnsafeCharacter,
    #[error("unsupported link scheme {0:?}")]
    UnsupportedScheme(String),
    #[error("invalid link")]
    Invalid,
    #[error("docs link must start with /docs/")]
    OutsideDocs,
    #[error("docs link must not contain parent traversal")]
    ParentTraversal,
}

pub fn classify_link(raw_link: &str) -> Result<LinkTarget, LinkError> {
    let trimmed = raw_link.trim();
    if trimmed.is_empty() {
        return Err(LinkError::Empty);
    }
    if trimmed.chars().any(|value| value.is_control()) {
        return Err(LinkError::UnsafeCharacter);
    }
    if let Ok(url) = tauri::Url::parse(trimmed) {
        return match url.scheme() {
            "http" | "https" => Ok(LinkTarget::External(url.to_string())),
            scheme => Err(LinkError::UnsupportedScheme(scheme.to_owned())),
        };
    }
    normalize_docs_link(trimmed).map(LinkTarget::Docs)
}

fn normalize_docs_link(raw_link: &str) -> Result<String, LinkError> {
    let (without_fragment, fragment) = split_once(raw_link, '#');
    let (path, query) = split_once(without_fragment, '?');
    let decoded = percent_decode_path(path)?;
    if decoded.split('/').any(|segment| segment == "..") {
        return Err(LinkError::ParentTraversal);
    }
    let mut segments = Vec::new();
    for segment in path.trim_start_matches('/').split('/') {
        match segment {
            "" | "." => {}
            ".." => return Err(LinkError::ParentTraversal),
            value => segments.push(value),
        }
    }
    if segments.first().copied() != Some("docs") {
        return Err(LinkError::OutsideDocs);
    }
    if segments.last().copied() == Some("index.html") {
        segments.pop();
    }
    let mut normalized = format!("/{}", segments.join("/"));
    if normalized == "/docs" || path.ends_with('/') || path.ends_with("/index.html") {
        normalized.push('/');
    }
    if let Some(query) = query {
        normalized.push('?');
        normalized.push_str(query);
    }
    if let Some(fragment) = fragment {
        normalized.push('#');
        normalized.push_str(fragment);
    }
    Ok(normalized)
}

fn percent_decode_path(path: &str) -> Result<String, LinkError> {
    let mut decoded = String::with_capacity(path.len());
    let bytes = path.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] != b'%' {
            decoded.push(char::from(bytes[index]));
            index += 1;
            continue;
        }
        if index + 2 >= bytes.len() {
            return Err(LinkError::Invalid);
        }
        let high = hex_value(bytes[index + 1]).ok_or(LinkError::Invalid)?;
        let low = hex_value(bytes[index + 2]).ok_or(LinkError::Invalid)?;
        decoded.push(char::from((high << 4) | low));
        index += 3;
    }
    Ok(decoded)
}

fn hex_value(byte: u8) -> Option<u8> {
    match byte {
        b'0'..=b'9' => Some(byte - b'0'),
        b'a'..=b'f' => Some(byte - b'a' + 10),
        b'A'..=b'F' => Some(byte - b'A' + 10),
        _ => None,
    }
}

fn split_once(value: &str, delimiter: char) -> (&str, Option<&str>) {
    value
        .split_once(delimiter)
        .map_or((value, None), |(left, right)| (left, Some(right)))
}
