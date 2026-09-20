/// Go `model.ApprovalResolutionSummary` for a denied approval: the ADK
/// continuation never executes the rejected tool, so the reply is rendered
/// locally from the resolved approval (falling back to the denied call's name).
fn denied_approval_summary(payload: &Value) -> String {
    let from_approval = payload
        .get("pendingApprovals")
        .and_then(Value::as_array)
        .and_then(|approvals| approvals.first())
        .and_then(|approval| approval.get("toolName"))
        .and_then(Value::as_str);
    let tool_name = from_approval
        .or_else(|| {
            payload
                .get("toolCalls")
                .and_then(Value::as_array)
                .and_then(|calls| {
                    calls.iter().find(|call| {
                        call.get("status")
                            .and_then(Value::as_str)
                            .is_some_and(|status| status.eq_ignore_ascii_case("DENIED"))
                    })
                })
                .and_then(|call| call.get("name"))
                .and_then(Value::as_str)
        })
        .unwrap_or_default()
        .trim();
    format!("已拒绝工具调用 `{tool_name}`。本次 run 已结束，未执行该操作。")
}
