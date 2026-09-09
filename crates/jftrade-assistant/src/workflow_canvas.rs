//! Canvas Workflow DAG definition, compiler, and node run models.

use std::collections::{BTreeMap, BTreeSet};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use thiserror::Error;

/// Directed graph describing a canvas workflow topology.
#[derive(Clone, Debug, Default, Deserialize, Serialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowCanvasGraph {
    #[serde(default)]
    pub version: String,
    #[serde(default)]
    pub nodes: Vec<WorkflowCanvasNode>,
    #[serde(default)]
    pub edges: Vec<WorkflowCanvasEdge>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub viewport: Option<Value>,
}

impl WorkflowCanvasGraph {
    /// Legacy single-prompt workflows use the same durable node executor.
    pub fn single_agent() -> Self {
        Self {
            nodes: ["trigger", "start", "agent", "monitor"]
                .into_iter()
                .map(|kind| WorkflowCanvasNode {
                    id: kind.to_owned(),
                    node_type: kind.to_owned(),
                    ..Default::default()
                })
                .collect(),
            edges: [
                ("trigger", "start"),
                ("start", "agent"),
                ("agent", "monitor"),
            ]
            .into_iter()
            .map(|(source, target)| WorkflowCanvasEdge {
                source: source.to_owned(),
                target: target.to_owned(),
                ..Default::default()
            })
            .collect(),
            ..Default::default()
        }
    }
}

/// A single node in a canvas workflow graph.
#[derive(Clone, Debug, Default, Deserialize, Serialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowCanvasNode {
    pub id: String,
    #[serde(rename = "type")]
    pub node_type: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub position: Option<Value>,
    #[serde(default)]
    pub data: BTreeMap<String, Value>,
}

impl WorkflowCanvasNode {
    /// Returns the string value associated with a key in node `data`.
    pub fn get_data_str(&self, key: &str) -> Option<&str> {
        self.data
            .get(key)
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|s| !s.is_empty())
    }

    /// Resolves the node's display title, falling back to label or node ID.
    pub fn title(&self) -> String {
        self.get_data_str("title")
            .or_else(|| self.get_data_str("label"))
            .unwrap_or(&self.id)
            .to_owned()
    }
}

/// A directed edge connecting two nodes in a canvas workflow graph.
#[derive(Clone, Debug, Default, Deserialize, Serialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowCanvasEdge {
    #[serde(default)]
    pub id: String,
    pub source: String,
    pub target: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub source_handle: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub target_handle: Option<String>,
}

/// Durable execution record for a single workflow node.
#[derive(Clone, Debug, Deserialize, Serialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowNodeRun {
    pub node_id: String,
    pub node_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub started_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub finished_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub inputs: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub outputs: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

/// Validation and compilation errors for canvas workflow graphs.
#[derive(Debug, Error, Eq, PartialEq)]
pub enum CanvasCompilerError {
    #[error("workflow canvas node id is required")]
    EmptyNodeId,
    #[error("workflow canvas duplicate node id {0}")]
    DuplicateNodeId(String),
    #[error("workflow canvas node {id} has unsupported type {node_type}")]
    UnsupportedNodeType { id: String, node_type: String },
    #[error("workflow canvas edge {id} requires valid source and target")]
    InvalidEdge { id: String },
    #[error("workflow canvas edge {0} connects node to itself")]
    SelfLoop(String),
    #[error("workflow canvas edge {edge} references unknown source {source_id}")]
    UnknownSource { edge: String, source_id: String },
    #[error("workflow canvas edge {edge} references unknown target {target_id}")]
    UnknownTarget { edge: String, target_id: String },
    #[error("workflow canvas contains a cycle")]
    Cycle,
    #[error("workflow canvas agent node {0} is not reachable from start or trigger")]
    UnreachableAgentNode(String),
    #[error("workflow canvas contains no executable agent nodes")]
    NoExecutableNodes,
}

/// Compiler that performs structural validation, cycle detection, and topological sorting.
#[derive(Debug)]
pub struct CanvasCompiler<'a> {
    graph: &'a WorkflowCanvasGraph,
    nodes: BTreeMap<String, &'a WorkflowCanvasNode>,
    types: BTreeMap<String, String>,
    out_edges: BTreeMap<String, Vec<String>>,
    in_edges: BTreeMap<String, Vec<String>>,
}

impl<'a> CanvasCompiler<'a> {
    /// Builds and validates indices for the given canvas graph.
    pub fn new(graph: &'a WorkflowCanvasGraph) -> Result<Self, CanvasCompilerError> {
        let mut nodes = BTreeMap::new();
        let mut types = BTreeMap::new();
        let mut out_edges = BTreeMap::new();
        let mut in_edges = BTreeMap::new();

        for node in &graph.nodes {
            let id = node.id.trim().to_owned();
            if id.is_empty() {
                return Err(CanvasCompilerError::EmptyNodeId);
            }
            if nodes.insert(id.clone(), node).is_some() {
                return Err(CanvasCompilerError::DuplicateNodeId(id));
            }

            let mut kind = node.node_type.trim().to_ascii_lowercase();
            if kind.is_empty()
                && let Some(data_type) = node.get_data_str("type")
            {
                kind = data_type.to_ascii_lowercase();
            }

            match kind.as_str() {
                "trigger" | "start" | "agent" | "monitor" => {
                    types.insert(id.clone(), kind);
                }
                _ => {
                    return Err(CanvasCompilerError::UnsupportedNodeType {
                        id,
                        node_type: node.node_type.clone(),
                    });
                }
            }
            out_edges.insert(id.clone(), Vec::new());
            in_edges.insert(id, Vec::new());
        }

        for edge in &graph.edges {
            let source = edge.source.trim();
            let target = edge.target.trim();
            if source.is_empty() || target.is_empty() {
                return Err(CanvasCompilerError::InvalidEdge {
                    id: edge.id.clone(),
                });
            }
            if source == target {
                return Err(CanvasCompilerError::SelfLoop(edge.id.clone()));
            }
            if !nodes.contains_key(source) {
                return Err(CanvasCompilerError::UnknownSource {
                    edge: edge.id.clone(),
                    source_id: source.to_owned(),
                });
            }
            if !nodes.contains_key(target) {
                return Err(CanvasCompilerError::UnknownTarget {
                    edge: edge.id.clone(),
                    target_id: target.to_owned(),
                });
            }

            let targets = out_edges.get_mut(source).expect("source in map");
            if !targets.iter().any(|t| t == target) {
                targets.push(target.to_owned());
                in_edges
                    .get_mut(target)
                    .expect("target in map")
                    .push(source.to_owned());
            }
        }

        Ok(Self {
            graph,
            nodes,
            types,
            out_edges,
            in_edges,
        })
    }

    /// Compiles the graph into a deterministic topological execution order and validates reachability.
    pub fn compile(&self) -> Result<Vec<String>, CanvasCompilerError> {
        let order = self.topological_order()?;
        self.validate_reachability(&order)?;
        Ok(order)
    }

    /// Computes Kahn's topological sort with deterministic lexicographical tie-breaking.
    pub fn topological_order(&self) -> Result<Vec<String>, CanvasCompilerError> {
        let mut indegree = BTreeMap::new();
        for id in self.nodes.keys() {
            indegree.insert(id.clone(), 0usize);
        }
        for targets in self.out_edges.values() {
            for target in targets {
                *indegree.get_mut(target).expect("node exists") += 1;
            }
        }

        let mut ready: Vec<String> = indegree
            .iter()
            .filter_map(|(id, &deg)| if deg == 0 { Some(id.clone()) } else { None })
            .collect();
        ready.sort();

        let mut order = Vec::with_capacity(self.nodes.len());
        while !ready.is_empty() {
            let current = ready.remove(0);
            order.push(current.clone());

            let mut targets = self.out_edges.get(&current).cloned().unwrap_or_default();
            targets.sort();
            for target in targets {
                let entry = indegree.get_mut(&target).expect("node exists");
                *entry -= 1;
                if *entry == 0 {
                    ready.push(target);
                    ready.sort();
                }
            }
        }

        if order.len() != self.nodes.len() {
            return Err(CanvasCompilerError::Cycle);
        }
        Ok(order)
    }

    /// Validates that all agent nodes are reachable from trigger or start nodes.
    pub fn validate_reachability(&self, order: &[String]) -> Result<(), CanvasCompilerError> {
        let mut reachable = BTreeSet::new();
        let mut queue = Vec::new();

        for (id, kind) in &self.types {
            if matches!(kind.as_str(), "trigger" | "start") {
                reachable.insert(id.clone());
                queue.push(id.clone());
            }
        }

        while let Some(curr) = queue.pop() {
            if let Some(targets) = self.out_edges.get(&curr) {
                for next in targets {
                    if reachable.insert(next.clone()) {
                        queue.push(next.clone());
                    }
                }
            }
        }

        let mut has_agent = false;
        for id in order {
            if self.types.get(id).map(String::as_str) == Some("agent") {
                has_agent = true;
                if !reachable.contains(id) {
                    return Err(CanvasCompilerError::UnreachableAgentNode(id.clone()));
                }
            }
        }

        if !has_agent {
            return Err(CanvasCompilerError::NoExecutableNodes);
        }

        Ok(())
    }

    /// Retrieves a node by ID.
    pub fn node(&self, id: &str) -> Option<&'a WorkflowCanvasNode> {
        self.nodes.get(id).copied()
    }

    /// Retrieves the normalized node type for a given node ID.
    pub fn node_type(&self, id: &str) -> Option<&str> {
        self.types.get(id).map(String::as_str)
    }

    /// Returns the immediate predecessor IDs for a node.
    pub fn incoming_edges(&self, id: &str) -> &[String] {
        self.in_edges.get(id).map(Vec::as_slice).unwrap_or(&[])
    }

    /// Returns the immediate successor IDs for a node.
    pub fn outgoing_edges(&self, id: &str) -> &[String] {
        self.out_edges.get(id).map(Vec::as_slice).unwrap_or(&[])
    }

    /// Accesses the underlying graph.
    pub fn graph(&self) -> &'a WorkflowCanvasGraph {
        self.graph
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn make_node(id: &str, node_type: &str) -> WorkflowCanvasNode {
        WorkflowCanvasNode {
            id: id.to_owned(),
            node_type: node_type.to_owned(),
            position: None,
            data: BTreeMap::new(),
        }
    }

    fn make_edge(id: &str, source: &str, target: &str) -> WorkflowCanvasEdge {
        WorkflowCanvasEdge {
            id: id.to_owned(),
            source: source.to_owned(),
            target: target.to_owned(),
            source_handle: None,
            target_handle: None,
        }
    }

    #[test]
    fn test_canvas_linear_chain() {
        let graph = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![
                make_node("monitor-node", "monitor"),
                make_node("start-node", "start"),
                make_node("agent-node", "agent"),
            ],
            edges: vec![
                make_edge("e1", "start-node", "agent-node"),
                make_edge("e2", "agent-node", "monitor-node"),
            ],
            viewport: None,
        };

        let compiler = CanvasCompiler::new(&graph).expect("compiler initialized");
        let order = compiler.compile().expect("compilation succeeds");
        assert_eq!(order, vec!["start-node", "agent-node", "monitor-node"]);
        assert_eq!(compiler.incoming_edges("agent-node"), &["start-node"]);
        assert_eq!(compiler.outgoing_edges("agent-node"), &["monitor-node"]);
    }

    #[test]
    fn test_canvas_diamond_graph_deterministic_tie_breaking() {
        // Start -> Agent B, Agent A -> Monitor
        // Tie breaking should put Agent A before Agent B due to sorted order
        let graph = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![
                make_node("start", "start"),
                make_node("agent-b", "agent"),
                make_node("agent-a", "agent"),
                make_node("monitor", "monitor"),
            ],
            edges: vec![
                make_edge("e1", "start", "agent-b"),
                make_edge("e2", "start", "agent-a"),
                make_edge("e3", "agent-b", "monitor"),
                make_edge("e4", "agent-a", "monitor"),
            ],
            viewport: None,
        };

        let compiler = CanvasCompiler::new(&graph).expect("compiler initialized");
        let order = compiler.compile().expect("compilation succeeds");
        assert_eq!(order, vec!["start", "agent-a", "agent-b", "monitor"]);
    }

    #[test]
    fn test_canvas_cycle_detection() {
        let graph = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![
                make_node("start", "start"),
                make_node("node-1", "agent"),
                make_node("node-2", "agent"),
            ],
            edges: vec![
                make_edge("e1", "start", "node-1"),
                make_edge("e2", "node-1", "node-2"),
                make_edge("e3", "node-2", "node-1"),
            ],
            viewport: None,
        };

        let compiler = CanvasCompiler::new(&graph).expect("compiler initialized");
        let err = compiler.compile().expect_err("must detect cycle");
        assert_eq!(err, CanvasCompilerError::Cycle);
    }

    #[test]
    fn test_canvas_self_loop() {
        let graph = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![make_node("start", "start"), make_node("agent", "agent")],
            edges: vec![make_edge("e1", "agent", "agent")],
            viewport: None,
        };

        let err = CanvasCompiler::new(&graph).expect_err("must reject self-loop");
        assert_eq!(err, CanvasCompilerError::SelfLoop("e1".to_owned()));
    }

    #[test]
    fn test_canvas_unreachable_agent() {
        let graph = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![
                make_node("start", "start"),
                make_node("agent-1", "agent"),
                make_node("agent-orphan", "agent"),
            ],
            edges: vec![make_edge("e1", "start", "agent-1")],
            viewport: None,
        };

        let compiler = CanvasCompiler::new(&graph).expect("compiler initialized");
        let err = compiler
            .compile()
            .expect_err("must reject unreachable agent");
        assert_eq!(
            err,
            CanvasCompilerError::UnreachableAgentNode("agent-orphan".to_owned())
        );
    }

    #[test]
    fn test_canvas_no_executable_nodes() {
        let graph = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![make_node("start", "start"), make_node("monitor", "monitor")],
            edges: vec![make_edge("e1", "start", "monitor")],
            viewport: None,
        };

        let compiler = CanvasCompiler::new(&graph).expect("compiler initialized");
        let err = compiler.compile().expect_err("must detect missing agent");
        assert_eq!(err, CanvasCompilerError::NoExecutableNodes);
    }

    #[test]
    fn test_canvas_invalid_node_and_edge_inputs() {
        let empty_node = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![make_node("   ", "start")],
            edges: Vec::new(),
            viewport: None,
        };
        assert_eq!(
            CanvasCompiler::new(&empty_node).unwrap_err(),
            CanvasCompilerError::EmptyNodeId
        );

        let dup_nodes = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![make_node("n1", "start"), make_node("n1", "agent")],
            edges: Vec::new(),
            viewport: None,
        };
        assert_eq!(
            CanvasCompiler::new(&dup_nodes).unwrap_err(),
            CanvasCompilerError::DuplicateNodeId("n1".to_owned())
        );

        let unsupported_type = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![make_node("n1", "invalid-type")],
            edges: Vec::new(),
            viewport: None,
        };
        assert_eq!(
            CanvasCompiler::new(&unsupported_type).unwrap_err(),
            CanvasCompilerError::UnsupportedNodeType {
                id: "n1".to_owned(),
                node_type: "invalid-type".to_owned(),
            }
        );

        let unknown_src = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![make_node("n1", "start")],
            edges: vec![make_edge("e1", "unknown", "n1")],
            viewport: None,
        };
        assert_eq!(
            CanvasCompiler::new(&unknown_src).unwrap_err(),
            CanvasCompilerError::UnknownSource {
                edge: "e1".to_owned(),
                source_id: "unknown".to_owned(),
            }
        );

        let unknown_dst = WorkflowCanvasGraph {
            version: "1.0".to_owned(),
            nodes: vec![make_node("n1", "start")],
            edges: vec![make_edge("e1", "n1", "unknown")],
            viewport: None,
        };
        assert_eq!(
            CanvasCompiler::new(&unknown_dst).unwrap_err(),
            CanvasCompilerError::UnknownTarget {
                edge: "e1".to_owned(),
                target_id: "unknown".to_owned(),
            }
        );
    }

    #[test]
    fn test_node_title_and_data_helpers() {
        let mut node = make_node("node-1", "agent");
        assert_eq!(node.title(), "node-1");

        node.data.insert("label".to_owned(), json!("Custom Label"));
        assert_eq!(node.title(), "Custom Label");

        node.data.insert("title".to_owned(), json!("Custom Title"));
        assert_eq!(node.title(), "Custom Title");
    }
}
