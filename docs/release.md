# 发布说明

## 目标形态

Source-First Monorepo + Release Artifacts

- 仓库只放源码和可复现脚本
- 真正可运行产物通过 Release 构建得到
- 不把本地运行态、用户态数据放进 git 历史

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
