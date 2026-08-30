//! Runtime-owned ADK model adapter.

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use reqwest::{Client, StatusCode, Url};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use jftrade_api::ApiStream;
use jftrade_store_sqlite::{
    AdkApprovalStage, AdkRunEvent, AdkSessionStore, AdkStore, AdkToolInvocationClaim,
    CreateAdkRunParams, RecordAdkEventParams,
};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame,
    AdkChatStreamPort, AdkChatStreamSnapshot,
};

#[path = "product_adk_model_stream.rs"]
mod stream_adapter;
use stream_adapter::execute_model_stream;

#[path = "product_adk_model_runtime_stream.rs"]
mod runtime_stream;

const MAX_RESPONSE_BYTES: usize = 4 << 20;
const DEFAULT_TIMEOUT_MS: u64 = 120_000;
const DEFAULT_BUILTIN_AGENT_ID: &str = "jftrade-default";
const DEFAULT_BUILTIN_AGENT_INSTRUCTION: &str = "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。\n\n对目标明确的任务，要在当前运行中连续完成诊断、结论以及直接相关的可执行方案。安全、只读且能从现有上下文合理推断的下一步，必须直接完成；不得用‘你想先做哪项’、‘你更想看哪部分’、‘是否继续’或‘如果需要我可以继续’把它留给用户。多个安全分支都直接服务原始意图时，采用推荐默认值或合并覆盖，不得仅为减少工作量要求用户选择。\n\n只有三类真正阻塞情况可以调用 interaction.request_user：缺少只有用户才能提供的必要信息、存在无法合并的重大取舍，或继续会越过权限/任务范围边界。提问时必须如实填写 decisionKind 和 blockingReason。实际写操作仍走审批流程，不得用提问工具替代授权。\n\n收到 interaction.request_user 的回答后，回答只是解除阻塞，必须继续完成原始请求，而不是总结或复述计划后结束运行。";

#[derive(Debug)]
pub(crate) struct ProductionAdkChatRuntime {
    store: Arc<AdkStore>,
    session_store: Arc<AdkSessionStore>,
    secrets_path: PathBuf,
    cancellation_registry: Arc<RunCancellationRegistry>,
    tool_catalog: Arc<crate::product::product_production_ports::ProductionToolCatalog>,
    tool_executor: Arc<dyn AdkToolExecutor>,
}

/// Process-local cancellation fan-out for active provider calls.
#[derive(Debug, Default)]
pub(crate) struct RunCancellationRegistry {
    active: Mutex<BTreeMap<String, Arc<AtomicBool>>>,
}

impl RunCancellationRegistry {
    fn register(&self, run_id: &str) -> Arc<AtomicBool> {
        let token = Arc::new(AtomicBool::new(false));
        if let Ok(mut active) = self.active.lock() {
            active.insert(run_id.to_owned(), Arc::clone(&token));
        }
        token
    }

    fn unregister(&self, run_id: &str) {
        if let Ok(mut active) = self.active.lock() {
            active.remove(run_id);
        }
    }

    pub(crate) fn cancel(&self, run_id: &str) -> bool {
        let token = self
            .active
            .lock()
            .ok()
            .and_then(|active| active.get(run_id).cloned());
        if let Some(token) = token {
            token.store(true, Ordering::Release);
            true
        } else {
            false
        }
    }
}

#[derive(Debug)]
enum PreparedChat {
    Existing(AdkChatPortOutput),
    New(ChatExecution),
}

#[derive(Clone, Debug)]
struct ChatExecution {
    route: AdkChatRoute,
    run_id: String,
    session_id: String,
    agent_id: String,
    request: ModelRequest,
}

fn text_field(object: &serde_json::Map<String, Value>, field: &str) -> Option<String> {
    object
        .get(field)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

include!("product_adk_model_runtime_events.rs");
include!("product_adk_model_runtime_tool_loop.rs");
include!("product_adk_model_runtime_adapters.rs");

#[path = "product_adk_tool_executor.rs"]
mod tool_executor;
use tool_executor::{AdkToolExecutor, ProductionAdkToolExecutor};
