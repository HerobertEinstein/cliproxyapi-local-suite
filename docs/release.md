# 发布说明

## 目标形态

Source-First Monorepo + Release Artifacts

- 仓库只放源码和可复现脚本
- 真正可运行产物通过 Release 构建得到
- 不把本地运行态、用户态数据放进 git 历史

## 建议发布流程

1. 在干净环境运行后端与 WebUI 构建
2. 运行定向测试
3. 生成 Windows 发布包
4. 将发布包作为 Release 附件上传

## 发布包建议内容

- `runtime/app/cpa.exe`
- `webui/dist/management.html`
- `ops/windows/*.ps1`
- `backend/config.example.yaml`
- `README.md`

## 发布前额外检查

- 确认源码中没有硬编码 OAuth Client ID / Client Secret
- 确认认证文件、浏览器 profile、Playwright 输出未进入 git
- 确认发布依赖的敏感值全部通过本地环境变量注入
