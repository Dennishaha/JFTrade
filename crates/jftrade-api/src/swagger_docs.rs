//! Offline API documentation panel served under `/swagger`.
//!
//! Parity baseline `go:452dea11`: `internal/app/apiserver/servercore/openapi.go`
//! mounted an embedded Swagger UI behind the same browser-access policy as the
//! API routes (`server_auth.go` authenticates `/swagger` paths too):
//! `/swagger` and `/swagger/` redirect to `/swagger/index.html`, the page loads
//! no external origin, and `/swagger/doc.json` returns the frozen Swagger 2.0
//! contract. The Rust transport keeps those URLs, the redirect shape, the
//! offline guarantee and the frozen contract, and renders the reference with a
//! first-party viewer instead of the Go library's bundled swagger-ui assets.

use axum::body::Body;
use axum::http::header::LOCATION;
use axum::http::{HeaderValue, Method, Response, StatusCode, Uri};

use crate::envelope::{body_response, empty_response};

const DOCUMENTATION_ENTRY: &str = "/swagger/index.html";

/// Frozen Swagger 2.0 contract served at `/swagger/doc.json`.
///
/// The document is injected by the composition root so the transport crate
/// never reads the filesystem and the served bytes stay identical to the
/// contract source validated by `pnpm run check:contracts`.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SwaggerDocs {
    document: Vec<u8>,
}

impl SwaggerDocs {
    /// Wrap the frozen Swagger 2.0 contract document.
    pub fn new(document: impl Into<Vec<u8>>) -> Self {
        Self {
            document: document.into(),
        }
    }

    /// Answer a `/swagger` documentation request.
    ///
    /// Returns `None` for anything outside the documentation surface — other
    /// methods and unknown asset paths keep the standard transport envelope.
    pub(crate) fn response(&self, method: &Method, uri: &Uri) -> Option<Response<Body>> {
        if !matches!(*method, Method::GET | Method::HEAD) {
            return None;
        }
        let path = uri.path();
        if path == "/swagger" || path == "/swagger/" {
            return Some(redirect_to_documentation());
        }
        let (content_type, bytes) = match path {
            "/swagger/index.html" => ("text/html; charset=utf-8", INDEX_HTML.as_bytes()),
            "/swagger/docs.css" => ("text/css; charset=utf-8", DOCS_CSS.as_bytes()),
            "/swagger/docs.js" => ("application/javascript; charset=utf-8", DOCS_JS.as_bytes()),
            "/swagger/doc.json" => ("application/json; charset=utf-8", self.document.as_slice()),
            _ => return None,
        };
        Some(if *method == Method::HEAD {
            body_response(StatusCode::OK, content_type, Body::empty())
        } else {
            body_response(StatusCode::OK, content_type, bytes.to_vec())
        })
    }
}

fn redirect_to_documentation() -> Response<Body> {
    let mut response = empty_response(StatusCode::TEMPORARY_REDIRECT);
    response
        .headers_mut()
        .insert(LOCATION, HeaderValue::from_static(DOCUMENTATION_ENTRY));
    response
}

/// The panel is a single static page: it links the local stylesheet and viewer
/// and never resolves an external origin, so the documentation stays readable
/// on an offline desktop.
const INDEX_HTML: &str = r#"<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>JFTrade 调试 API 文档</title>
<link rel="stylesheet" href="/swagger/docs.css">
</head>
<body>
<header class="page-header">
<div class="brand">JFTRADE</div>
<h1>调试 API 文档</h1>
<p>离线渲染 <code>/swagger/doc.json</code>（Swagger 2.0 冻结契约），不加载任何外部 CDN 资源。</p>
<label class="filter">过滤<input id="filter" type="search" placeholder="路径或方法，例如 market-data" autocomplete="off"></label>
</header>
<main id="api-reference" class="reference"></main>
<script src="/swagger/docs.js"></script>
</body>
</html>
"#;

const DOCS_CSS: &str = r#"
:root{color-scheme:light dark;--surface:#ffffff;--page:#f6f7f9;--ink:#1f2937;--muted:#5b6472;--line:#dfe3e8;--accent:#0f766e}
*{box-sizing:border-box}
body{margin:0;background:var(--page);color:var(--ink);font-family:system-ui,-apple-system,"Segoe UI",sans-serif}
.page-header{max-width:60rem;margin:0 auto;padding:2.5rem 1.5rem 1rem}
.brand{color:var(--accent);font-weight:700;letter-spacing:.16em}
h1{font-size:1.6rem;margin:.6rem 0}
.page-header p{margin:0 0 1rem;color:var(--muted);line-height:1.7}
.filter{display:flex;align-items:center;gap:.6rem;color:var(--muted);font-size:.9rem}
.filter input{flex:1;padding:.55rem .75rem;border:1px solid var(--line);border-radius:.5rem;background:var(--surface);color:inherit;font-size:.95rem}
.reference{max-width:60rem;margin:0 auto;padding:0 1.5rem 3rem}
.count{color:var(--muted);font-size:.85rem}
section{background:var(--surface);border:1px solid var(--line);border-radius:.75rem;margin:1rem 0;padding:1rem 1.25rem}
section h2{margin:.2rem 0 .8rem;font-size:1.05rem}
ul{list-style:none;margin:0;padding:0}
li{padding:.5rem 0;border-top:1px solid var(--line)}
li:first-child{border-top:0}
.method{display:inline-block;min-width:4.2rem;margin-right:.6rem;padding:.15rem .45rem;border-radius:.35rem;background:#e2e8f0;color:#334155;font:600 .75rem/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;text-align:center}
.method-get{background:#dcfce7;color:#166534}
.method-post{background:#dbeafe;color:#1d4ed8}
.method-put,.method-patch{background:#fef3c7;color:#92400e}
.method-delete{background:#fee2e2;color:#b91c1c}
code{font:400 .9rem/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}
li p{margin:.35rem 0 0;color:var(--muted);font-size:.85rem}
"#;

/// The viewer reads the frozen contract from the same origin and only builds
/// DOM nodes, so contract text is never interpreted as markup.
const DOCS_JS: &str = r##"
const reference = document.getElementById("api-reference");
const filter = document.getElementById("filter");
let operations = [];

function group(path) {
  const segments = path.split("/").filter(Boolean);
  return segments[2] ?? segments[0] ?? "api";
}

function render() {
  const needle = filter.value.trim().toLowerCase();
  const matches = operations.filter((operation) =>
    `${operation.method} ${operation.path} ${operation.summary}`.toLowerCase().includes(needle));
  reference.replaceChildren();
  const count = document.createElement("p");
  count.className = "count";
  count.textContent = `匹配 ${matches.length} / ${operations.length} 个操作`;
  reference.append(count);
  const groups = new Map();
  for (const operation of matches) {
    const key = group(operation.path);
    const entries = groups.get(key) ?? [];
    entries.push(operation);
    groups.set(key, entries);
  }
  for (const [key, entries] of groups) {
    const section = document.createElement("section");
    const heading = document.createElement("h2");
    heading.textContent = key;
    section.append(heading);
    const list = document.createElement("ul");
    for (const entry of entries) {
      const item = document.createElement("li");
      const badge = document.createElement("span");
      badge.className = `method method-${entry.method.toLowerCase()}`;
      badge.textContent = entry.method;
      const label = document.createElement("code");
      label.textContent = entry.path;
      item.append(badge, label);
      if (entry.summary) {
        const note = document.createElement("p");
        note.textContent = entry.summary;
        item.append(note);
      }
      list.append(item);
    }
    section.append(list);
    reference.append(section);
  }
}

fetch("/swagger/doc.json", { headers: { accept: "application/json" } })
  .then((response) => {
    if (!response.ok) throw new Error(`doc.json ${response.status}`);
    return response.json();
  })
  .then((contract) => {
    operations = [];
    for (const [path, methods] of Object.entries(contract.paths ?? {})) {
      for (const [method, operation] of Object.entries(methods ?? {})) {
        operations.push({
          path,
          method: method.toUpperCase(),
          summary: operation?.summary ?? "",
        });
      }
    }
    operations.sort((left, right) =>
      left.path.localeCompare(right.path) || left.method.localeCompare(right.method));
    filter.addEventListener("input", render);
    render();
  })
  .catch((error) => {
    reference.textContent = `无法加载 /swagger/doc.json：${error}`;
  });
"##;
