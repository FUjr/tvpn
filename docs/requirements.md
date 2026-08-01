# Tvpn 需求

## 项目规范

- 使用 Git 追踪；每项实现或修复独立提交，并用中文详细描述。
- 暂定项目名为 Tvpn；公开仓库为 `FUjr/tvpn`。
- 接口、数据库变化同步更新文档，功能变化同步更新 README。
- 工具、用户和 CI 脚本统一放在 `scripts/`。
- 前端调用接口统一从 `web/src/api/` 声明和导出。
- 删除已弃用接口和环境变量，不保留无效兼容代码。
- 应用通过 Docker 交付。

## 功能需求

- 服务端代理，浏览器登录后显示地址栏及下方代理页面。
- 支持本地登录和 LDAP，策略可绑定用户与 LDAP 组。
- 权限模式包括禁止内网、白名单、黑名单和禁止访问。
- 优先 Hook 浏览器 URL 与导航行为，其次监控动态 URL，最后由服务端重写可安全识别的 URL。
- 可拦截的 Fetch、XHR、EventSource 和 Beacon 请求强制通过复用 WebSocket；导航、早期资源和上传下载保留 HTTP 通道。
- 尽量兼容公网网站，但不承诺 WebRTC、WebTransport、DRM、浏览器扩展或任意直接 `window.location` 赋值。

