# 浏览器兼容性

## 首版支持

- HTTP/HTTPS 文档、重定向、表单、上传下载及常见 HTML/CSS 静态资源。
- Fetch、异步 XMLHttpRequest、EventSource、sendBeacon 和流式响应。
- `ws/wss` 文本帧、二进制帧、子协议、Cookie、Origin 和关闭状态。
- 链接、表单、`window.open`、History API、可用的 Navigation API 及动态 DOM URL。
- 服务端 HttpOnly Cookie 和脚本可见的 `document.cookie`。

## 已知边界

- 浏览器不允许可靠替换 `window.location` 的所有直接赋值；运行时通过事件捕获、静态改写和 iframe 防逃逸覆盖常见导航。
- 不支持同步 XHR、上游 Service Worker、WebTransport、WebRTC、DRM、浏览器扩展和非 HTTP/HTTPS/WS/WSS 协议。
- Web Worker 内部发起的请求仍使用代理 HTTP 路由，不进入页面级复用 WebSocket。
- 上游 CSP 和 SRI 会因代码注入与内容改写失效；Tvpn 会删除这些头和属性。
- 复用通道返回的 Fetch 响应不执行浏览器原生 CORS 隔离，最终可访问范围以 Tvpn 策略为准。

兼容性验收使用 `scripts/fixture` 和 Playwright，覆盖 Fetch、XHR、SSE、HttpOnly/脚本 Cookie、导航重定向，以及带子协议的文本和二进制 WebSocket。

