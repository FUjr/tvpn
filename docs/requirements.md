# Tvpn 需求

## 项目规范

- 使用 Git 追踪；每项实现或修复独立提交，并用中文详细描述。
- 暂定项目名为 Tvpn；公开仓库为 `FUjr/tvpn`。
- 接口、数据库变化同步更新文档，功能变化同步更新 README。
- 工具、用户和 CI 脚本统一放在 `scripts/`。
- 前端调用接口统一从 `web/src/api/` 声明和导出。
- 删除已弃用接口和环境变量，不保留无效兼容代码。
- 应用通过 Docker 交付。
- 管理员初始化默认使用已拉取或已构建的镜像，不得隐式重新构建；本地首次启动可显式请求构建。

## 功能需求

- 服务端代理，浏览器登录后显示地址栏及下方代理页面。
- 地址栏目标可省略 HTTP/HTTPS 协议；省略时自动补全为 `http://`，显式协议保持不变。
- 支持本地登录和 LDAP，策略可绑定用户与 LDAP 组。
- 管理员可维护 HTTP 和 SOCKS5 上游代理列表，并分别授权给本地/LDAP 用户和 LDAP 组；用户可选择任一有效授权代理，或不选代理而由服务端直连。
- 上游代理选择固定在浏览上下文中；切换出口必须关闭旧上下文，撤销授权或停用代理应立即阻止该上下文继续建立上游连接。
- 权限模式包括禁止内网、白名单、黑名单和禁止访问。
- 优先 Hook 浏览器 URL 与导航行为，其次监控动态 URL，最后由服务端重写可安全识别的 URL。
- 可拦截的 Fetch、XHR、EventSource 和 Beacon 请求强制通过复用 WebSocket；导航、早期资源和上传下载保留 HTTP 通道。
- 被代理页面创建的 `WebSocket` 必须由运行时 Hook 并经服务端建立 `ws/wss` 隧道，兼容子协议、文本/二进制帧、关闭码、Cookie 与 Origin。
- 允许管理域位于代理基础域之下，例如 `TVPN_APP_ORIGIN=https://vpn.proxy.example.com` 与 `TVPN_PROXY_BASE_DOMAIN=proxy.example.com`；服务端必须优先精确识别管理域，再匹配代理通配域。
- 尽量兼容公网网站，但不承诺 WebRTC、WebTransport、DRM、浏览器扩展或任意直接 `window.location` 赋值。
