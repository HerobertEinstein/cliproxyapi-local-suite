# 安全边界

## 可公开内容

- `backend/`、`webui/` 源码
- 示例配置
- 不含个人账号和环境绑定的启动脚本
- 公开使用文档

## 严禁入库内容

- 真实 `config.yaml`
- 管理密钥、客户端 API Key、OAuth/认证文件
- `runtime/`、`state/`、`logs/`
- 浏览器 profile、调试输出、自动化缓存
- 个人导入脚本、私有上游同步脚本

## 公开版脚本约束

- 不从任何个人目录读取密钥
- 不自动回填个人工作流配置
- 不使用固定绝对路径
- 默认只服务本机 `127.0.0.1`
- 内置 OAuth Client ID / Client Secret 必须通过环境变量注入，不得硬编码进源码或提交到仓库

## 发布前检查

- 确认没有 `.exe`、`dist/`、`node_modules/`
- 确认没有真实 `config/config.yaml`
- 确认没有 `management-key.txt`、`client-api-key.txt`
- 确认没有测试日志、浏览器缓存、Playwright 输出
