// 按需版 Monaco 入口，通过 vite.config.ts 的 resolve.alias 替换对 "monaco-editor"
// 裸导入（仅运行时；类型仍来自 monaco-editor 官方声明）。
//
// monaco-editor@0.56 的完整入口（esm/vs/index.js）会静态引入：
//   - monaco-lsp-client（lsp 导出，本项目不使用）
//   - 80+ basic-language 定义（abap/cpp/python/... 各自再产生懒加载 chunk）
//   - css / html / json 语言服务（各自拖出一个独立 worker chunk）
// 这里只保留编辑器全部 contrib（与完整入口的 contrib 清单一致，行为不变）。
// JavaScript/TypeScript 语言定义与服务由 MonacoCodeEditor 在确有需要时按需加载，
// 使 Pine/JSON 编辑器不会把脚本语言服务带入自己的运行时依赖图。
// pine-v6 语言由 @/features/monacoEditorSupport 以 monarch 方式自定义注册，不受影响。

import "monaco-editor/features/register.all.js";
// register.all.js 相比完整入口（esm/vs/index.js）尾部缺少的 contrib，逐一补齐，
// 保持 suggest / find / 快捷键等编辑器行为与全量入口一致。
import "monaco-editor/editor/browser/coreCommands.js";
import "monaco-editor/editor/contrib/caretOperations/browser/caretOperations.js";
import "monaco-editor/editor/contrib/dropOrPasteInto/browser/copyPasteContribution.js";
import "monaco-editor/editor/contrib/find/browser/findController.js";
import "monaco-editor/editor/contrib/gotoSymbol/browser/goToCommands.js";
import "monaco-editor/editor/contrib/gotoError/browser/markerSelectionStatus.js";
import "monaco-editor/editor/contrib/semanticTokens/browser/documentSemanticTokens.js";
import "monaco-editor/editor/contrib/suggest/browser/suggestController.js";
import "monaco-editor/editor/common/standaloneStrings.js";
// 注：codicon-modifiers.css 已由 suggest/codeAction/quickAccess 等 contrib 间接引入，
// 且 monaco-editor 的 exports 映射不支持从包外直接引用 .css 子路径，故不重复引入。

export {
  CancellationTokenSource,
  Emitter,
  KeyCode,
  KeyMod,
  MarkerSeverity,
  MarkerTag,
  Position,
  Range,
  Selection,
  SelectionDirection,
  Token,
  Uri,
  editor,
  languages,
} from "monaco-editor/editor/editor.api.js";
