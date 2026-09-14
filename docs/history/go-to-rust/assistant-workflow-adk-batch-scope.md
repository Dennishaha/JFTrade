# Assistant、Workflow、ADK 领域对齐批次

本批逐项核对 6 条 Assistant/ADK 高风险 Go 测试：空集合序列化、chat stream
事件 wire、非法 session 路由、超大 offset 分页钳制、断线后的 stream 重放，
以及 legacy assistant route 边界。

| Go 测试 | Rust 证据 | 结论 |
| --- | --- | --- |
| `internal/api/assistant/adk_normalize_test.go:15` | `product_adk_read_tests::adk_read_fixture_preserves_empty_collections_as_json_arrays` | `[~]`/`partial`：只覆盖 runs/sessions 数组，缺 run detail 中 toolCalls/artifacts 空数组和真实 HTTP route |
| `internal/api/assistant/adk_routes_test.go:245` | `product_adk_chat_stream_product_tests::adk_chat_stream_routes_register_only_with_explicit_test_port` | `[~]`/`partial`：SSE frame 有证据，但 idle timeout 与完整 session/run/final 事件集合不同 |
| `internal/api/assistant/adk_routes_test.go:787` | `product_adk_read_tests::adk_read_dynamic_routes_validate_suffixes_and_identifiers` | `[~]`/`partial`：非法标识符矩阵有证据，缺 session 创建/缺失 404/malformed rename route |
| `internal/api/assistant/routes_payload_pagination_test.go:65` | `product_adk_read_tests::adk_read_routes_clamp_pagination_beyond_available_items` | `[~]`/`partial`：三个集合均断言空页 metadata；Go 通过 HTTP router，装配不同 |
| `internal/api/assistant/chat_transport_disconnect_test.go:55` | `product_adk_chat_stream_product_tests::adk_chat_stream_replays_retained_terminal_events_through_adk_read_after_restart` | `[~]`/`partial`：断线后重放与终态保留有证据，缺 transport 写失败/goroutine 清理细节 |
| `internal/api/assistant/adk_ops_test.go:448` | `jftrade-api::transport_contracts::legacy_assistant_chat_route_is_strictly_not_found` | `[~]`/`partial`：Rust 额外锁定 JSON content-type 与无 port dispatch，Go 仅断言 404 |

验证命令均写入 `manual-test-mappings.json`；本批没有把部分 SSE、fixture 或
route 证据升级为完整等价。后续优先补 approval/session workflow lease、取消
恢复和断线写失败回归测试。
