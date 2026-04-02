# 发布说明

## 目标形态

Source-First Monorepo + Release Artifacts

- 仓库只放源码和可复现脚本
- 真正可运行产物通过 Release 构建得到
- 不把本地运行态、用户态数据放进 git 历史

## 相对直接使用原版 CLIProxyAPI 仓库，这个发布版额外补了什么

这个 Local Suite 面向“Windows 本机可复现发布”做了额外整合，新增重点不在核心代理内核，而在发布外层：

- 把后端源码和管理面板源码一起收进同一仓库，形成 `backend/ + webui/` 的单仓库发布结构
- 增加 `ops/windows/` 下的本地启动、停止、配置同步、打开管理面板脚本，便于直接生成 Windows 本地发布包
- 增加根级发布与安全说明，明确哪些内容可以公开、哪些运行态和敏感文件必须留在仓库外
- 把管理面板来源区分为“运行时拉取”和“固定版随包分发”两种模式，便于在 Release 中做清晰取舍

因此，这个仓库比“直接发布原版后端源码”多的是本地套件化、脚本化和公开发布边界整理；核心功能能力仍以 `backend/` 中的 CLIProxyAPI 实现和 `webui/` 中的管理面板源码为基础。

## 当前仓库事实

- 当前公开仓库没有内置 `.github/workflows/release.yml`。
- 因此这里的发布流程以本地或外部 CI 手工构建为准，不应假设“打 tag 自动产出 Release 附件”已经存在。

## 建议发布流程

1. 在干净环境构建后端：

   ```powershell
   go -C .\backend build -o ..\runtime\app\cpa.exe .\cmd\server
   ```

2. 如果要随发布包附带固定版管理面板，再构建 WebUI：

   ```powershell
   npm --prefix .\webui ci
   npm --prefix .\webui run build
   ```

3. 按需选择管理面板分发方式：
   - 运行时下载：发布包里不附带前端静态文件，交由后端按 `remote-management.panel-github-repository` 拉取。
   - 固定版打包：把 `webui/dist/index.html` 重命名为 `management.html` 后再放入发布包。
4. 运行定向校验：

   ```powershell
   go -C .\backend test ./internal/auth/gemini ./internal/auth/antigravity ./internal/auth/iflow ./sdk/auth
   ```

5. 生成 Windows 发布包。
6. 将发布包作为 GitHub / Gitee Release 附件上传。

## 发布包建议内容

- `runtime/app/cpa.exe`
- `ops/windows/*.ps1`
- `backend/config.example.yaml`
- `README.md`
- `LICENSE`
- 可选：`management.html`（由 `webui/dist/index.html` 重命名得到）

## 发布前额外检查

- 确认源码中没有硬编码 OAuth Client ID / Client Secret
- 确认认证文件、浏览器 profile、Playwright 输出、`config/static/` 未进入 git
- 确认发布依赖的敏感值全部通过本地环境变量注入
- 如果发布包附带固定版 `management.html`，只放进 Release 附件，不回写到仓库历史
