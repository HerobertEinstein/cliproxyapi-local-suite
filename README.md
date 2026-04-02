# CLIProxyAPI Local Suite

面向 Windows 本机使用的 CLIProxyAPI 单仓库发布版。

这个仓库只保留三类内容：

- `backend/`：CLIProxyAPI 后端源码
- `webui/`：管理面板源码
- `ops/windows/`：公开可复现的本地启动、停止、配置脚本

不纳入仓库的内容：

- 二进制产物
- 真实配置、密钥、认证文件、日志、浏览器 profile
- 个人化导入脚本和第三方账号同步脚本

## 仓库结构

```text
backend/        Go 后端源码
webui/          React 管理面板源码
ops/windows/    Windows 本地运维脚本
docs/           公开使用说明与发布说明
scripts/        构建/打包/校验脚本（待补充）
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

内置 Gemini / Antigravity / iFlow OAuth 登录不会再使用仓库内置凭据；需要先按 `docs/config.md` 注入对应环境变量。

## 设计原则

- 默认仅监听 `127.0.0.1`
- 本地运行态全部落在仓库外的 `runtime/`、`config/`、`logs/` 目录
- 公开脚本不自动注入 management key，不捆绑个人工作流
- 公开仓库只提供可复现方法，不提交运行结果

## 许可证

根许可证沿用 MIT，聚合了上游后端与 WebUI 的 MIT 许可。
