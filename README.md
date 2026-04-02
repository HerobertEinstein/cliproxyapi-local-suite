# CLIProxyAPI Local Suite

面向 Windows 本机使用的 CLIProxyAPI 单仓库发布版。

## 相对直接使用原版 CLIProxyAPI，多了什么

这个 Local Suite 不是另起一套后端协议，而是在保留上游 CLIProxyAPI 后端与管理面板源码的基础上，额外补了面向 Windows 本机使用和公开发布的整套外层包装：

- 把 `backend/`、`webui/`、`ops/windows/`、`docs/` 收敛到同一个仓库里，便于本地复现、打包和发布
- 额外提供 `ops/windows/` 下的一键脚本，用于本地配置同步、启动、停止和打开管理面板
- 额外补齐根级 `README.md`、`docs/config.md`、`docs/release.md`、`docs/security.md`，把“如何运行、如何发布、哪些内容不能入仓”说清楚
- 额外明确 Source-First 的发布边界：仓库保留源码和可复现脚本，真实密钥、认证文件、运行态目录、日志和浏览器 profile 不入仓
- 额外把管理面板的两种分发方式说清楚：运行时由后端拉取 `management.html`，或者自行构建固定版再随 Release 分发

如果你关心的是核心代理能力、协议转换、OAuth、模型映射、路由与管理接口，这些仍以后端 `backend/` 中的 CLIProxyAPI 实现为准；这个仓库新增的重点是“本地套件化”和“公开发布整理”，不是重写核心内核。

这个仓库只保留四类内容：

- `backend/`：CLIProxyAPI 后端源码
- `webui/`：管理面板源码
- `ops/windows/`：公开可复现的本地启动、停止、配置脚本
- `docs/`：公开使用说明与发布说明

不纳入仓库的内容：

- 二进制产物
- 真实配置、密钥、认证文件、日志、浏览器 profile
- 个人化导入脚本和第三方账号同步脚本

## 后端新增：动态指针 / 逻辑模型组

当前后端已经补上 logical model groups 这套能力，用来把“客户端看到的模型名”和“真实上游 target model”拆开管理：

- `current` 是保留的**动态指针**，客户端可以长期只填 `current`
- `static` 是可增删的**静态组**，每个 `alias` 映射一个真实 `target`
- `current` 通过 `ref` 指向某个静态组，因此切换目标模型时不需要改客户端配置
- 每个静态组还可以定义 `reasoning`，决定是沿用客户端请求强度，还是由组内固定思考强度

仓库首页这里只做入口说明；完整字段、行为规则和 Management API 用法见：

- `backend/README_CN.md`
- `backend/README.md`
- `backend/README_JA.md`

## 仓库结构

```text
backend/        Go 后端源码
webui/          React 管理面板源码
ops/windows/    Windows 本地运维脚本
docs/           公开使用说明与发布说明
```

## 本地运行

1. 在 `backend/` 构建可执行文件：

   ```powershell
   go -C .\backend build -o ..\runtime\app\cpa.exe .\cmd\server
   ```

2. 生成本地配置：

   ```powershell
   .\ops\windows\Sync-CLIProxyAPI-Config.ps1
   ```

3. 启动服务：

   ```powershell
   .\ops\windows\Start-CLIProxyAPI.ps1
   ```

4. 打开管理面板：

   ```powershell
   .\ops\windows\Open-CLIProxyAPI-GUI.ps1
   ```

   这个脚本会先确保服务可用，再打开 `http://127.0.0.1:8317/management.html`。

内置 Gemini / Antigravity / iFlow OAuth 登录不会再使用仓库内置凭据；需要先按 `docs/config.md` 注入对应环境变量。

## 管理面板来源

- `http://127.0.0.1:8317/management.html` 由后端路由提供，不是直接读取仓库里的 `webui/` 源码目录。
- 默认情况下，后端会按 `remote-management.panel-github-repository` 拉取最新 `management.html`。
- 首次拉取后的本地文件默认落在 `config/static/management.html`；如果设置了 `WRITABLE_PATH`，则改为 `$env:WRITABLE_PATH/static/management.html`。
- `remote-management.disable-control-panel: true` 会关闭控制面板路由。
- `remote-management.disable-auto-update-panel: true` 会关闭后台自动更新；本地文件缺失时仍会按需补拉一次。
- `webui/` 目录主要用于二次开发或自建固定版前端，不是正常本地启动的前置条件。

## 设计原则

- 默认仅监听 `127.0.0.1`
- 本地运行态默认落在仓库根目录下未跟踪的 `runtime/`、`config/`、`logs/` 目录
- 管理面板缓存文件默认落在未跟踪的 `config/static/`
- 公开脚本不自动注入 management key，不捆绑个人工作流
- 公开仓库只提供可复现方法，不提交运行结果

## 许可证

根许可证沿用 MIT，聚合了上游后端与 WebUI 的 MIT 许可。
