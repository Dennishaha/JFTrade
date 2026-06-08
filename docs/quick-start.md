# 快速开始

## 开发态

```bash
npm install
npm run dev:web
```

如需打开文档开发站：

```bash
npm run dev:docs
```

用户访问入口保持为 `http://localhost:5173/docs/`。

## 发布态

```bash
./build-release.sh
```

Windows PowerShell:

```powershell
.\build-release.ps1
```

发布态默认从 GUI 同源路径提供文档：`/docs/`。
