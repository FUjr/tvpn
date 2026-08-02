# 浏览器兼容性

## 首版支持

- HTTP/HTTPS 文档、重定向、原生及 SPA 自管表单、上传下载及常见 HTML/CSS 静态资源。
- Fetch、异步 XMLHttpRequest、EventSource、sendBeacon 和流式响应。
- `ws/wss` 文本帧、二进制帧、子协议、Cookie、Origin 和关闭状态。
- 链接、表单、`window.open`、History API、可用的 Navigation API 及动态 DOM URL。
- 服务端 HttpOnly Cookie 和脚本可见的 `document.cookie`。

## 可选兼容模式

网址框旁的“兼容模式”默认关闭。遇到依赖多个 CDN Origin、频繁动态创建资源节点或集中写入 Cookie 的页面时，用户可以为当前浏览上下文打开它。切换时 Tvpn 会关闭旧上下文，并使用当前网址和出口重新打开页面。

启用后，Tvpn 会把上游 CORS 结果限定映射到发起请求的同一代理上下文，允许该上下文内因匿名 CORS 或预检而未携带代理 Cookie 的跨路由资源请求，补全预检响应，并对动态 URL 解析进行有界缓存、请求去重和并发限制，同时按顺序写入脚本 Cookie。无 Cookie 请求的 `Origin` 必须映射到同一活动上下文，并且不能访问 Tvpn 控制接口或执行文档导航；该模式不会绕过访问策略、直连 ACL 或上游代理授权，但会放宽当前浏览上下文内不同上游 Origin 之间的浏览器 CORS 隔离，因此仅应在普通模式无法正常加载页面时使用。

## 已知边界

- 浏览器不允许可靠替换 `window.location` 的所有直接赋值；运行时通过事件捕获、静态改写和 iframe 防逃逸覆盖常见导航。
- 不支持同步 XHR、上游 Service Worker、WebTransport、WebRTC、DRM、浏览器扩展和非 HTTP/HTTPS/WS/WSS 协议。
- Web Worker 内部发起的请求仍使用代理 HTTP 路由，不进入页面级复用 WebSocket。
- 上游 CSP 和 SRI 会因代码注入与内容改写失效；Tvpn 会删除这些头和属性。
- 复用通道返回的 Fetch 响应不执行浏览器原生 CORS 隔离，最终可访问范围以 Tvpn 策略为准。

兼容性验收使用 `scripts/fixture` 和 Playwright，覆盖 Fetch、XHR、SSE、HttpOnly/脚本 Cookie、SPA 自管表单、导航重定向，以及带子协议的文本和二进制 WebSocket。
