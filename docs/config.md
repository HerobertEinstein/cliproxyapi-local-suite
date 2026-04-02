# 配置说明

## 配置分层

- `backend/config.example.yaml`：上游示例模板
- `config/config.local.yaml`：本地实际运行配置
- `config/management-key.txt`：本地管理密钥
- `config/client-api-key.txt`：本地客户端访问密钥

这三个本地文件都不应提交。

## 公开版脚本行为

`ops/windows/Sync-CLIProxyAPI-Config.ps1` 会：

- 创建 `config/`
- 生成本地管理密钥与客户端 API Key
- 从 `backend/config.example.yaml` 生成 `config/config.local.yaml`
- 强制监听 `127.0.0.1`
- 写入本地 API Key

## 手工定制

如果你要接自己的上游，请只编辑：

- `config/config.local.yaml`

不要修改仓库中的 `backend/config.example.yaml` 来承载个人配置。

## 内置 OAuth 凭据注入

公开仓库不再内置 Gemini、Antigravity、iFlow 的 OAuth Client ID / Client Secret。

如果你要使用这三类内置登录流，请在启动后端前自行注入环境变量：

- `GEMINI_OAUTH_CLIENT_ID`
- `GEMINI_OAUTH_CLIENT_SECRET`
- `ANTIGRAVITY_OAUTH_CLIENT_ID`
- `ANTIGRAVITY_OAUTH_CLIENT_SECRET`
- `IFLOW_OAUTH_CLIENT_ID`
- `IFLOW_OAUTH_CLIENT_SECRET`

这些值只应存在于你的本地环境，不应写回仓库、示例配置或认证文件模板。
