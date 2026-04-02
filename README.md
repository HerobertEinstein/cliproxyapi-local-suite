# CLIProxyAPI Local Suite

面向 Windows 本机使用的 CLIProxyAPI 单仓库发布版。

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
