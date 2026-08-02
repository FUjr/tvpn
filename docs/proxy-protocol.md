# 代理协议

## HTTP 路由

- `bootstrap.<proxy-base>/__tvpn/bootstrap?ticket=...`：消费一次性票据并设置代理会话。
- `<route-id>.<proxy-base>/<path>`：`route-id` 映射到一个用户上下文和原始 HTTP/HTTPS Origin。

服务端保留请求方法和流式正文，删除 hop-by-hop 与 `X-Forwarded-*` 头，重建上游 Host、Origin、Referer 和 Cookie。响应侧保存并移除上游 `Set-Cookie`，重写 `Location`，删除阻止嵌入的 frame/CSP 响应头，并流式返回正文。

每个浏览上下文固定为服务端直连、HTTP 上游代理或 SOCKS5 上游代理之一。服务端直连可由管理员启用用户/LDAP 组 ACL，未获授权时不能创建直连上下文或建立直连套接字。HTTP 代理对 HTTP 与 HTTPS 目标都先建立 CONNECT 隧道；SOCKS5 和 CONNECT 的目标均为 Tvpn 已解析、检查的 IP 和端口，原始域名只用于 HTTP Host 与 TLS SNI。页面 HTTP、多路复用请求和原生 WebSocket 共用同一出口选择。

## 注入运行时

HTML 解析器在目标脚本之前注入配置和 `/__tvpn/runtime.js`。服务端先改写 HTML URL 属性、`srcset`、Meta Refresh、内联样式和 CSS `url()`；运行时再处理 Fetch、XHR、EventSource、Beacon、动态 DOM、链接、表单、`window.open` 和 Navigation API。

可改写的 HTML/CSS 正文上限为 16 MiB。改写后移除上游 CSP、X-Frame-Options、内容长度、ETag 和受影响资源的 SRI 属性。

## 请求复用 WebSocket

`/__tvpn/mux` 使用二进制帧。每帧前 6 字节为版本 `1`、帧类型和大端 uint32 流 ID；剩余部分是 JSON 元数据或原始正文块。

| 类型 | 值 | 方向 | 内容 |
| --- | --- | --- | --- |
| request/start | 1 | 浏览器到服务端 | method、绝对 URL、headers JSON |
| request/chunk | 2 | 浏览器到服务端 | 原始正文块 |
| request/end | 3 | 浏览器到服务端 | 空 |
| request/cancel | 4 | 浏览器到服务端 | 空 |
| response/start | 11 | 服务端到浏览器 | status、statusText、headers、可见 Cookie JSON |
| response/chunk | 12 | 服务端到浏览器 | 原始正文块 |
| response/end | 13 | 服务端到浏览器 | 空 |
| response/error | 14 | 服务端到浏览器 | UTF-8 错误摘要 |

单请求正文上限为 32 MiB。Fetch、异步 XHR、EventSource 和 Beacon 强制使用该通道；通道不可用时不会绕过代理直连。

## 原生 WebSocket

运行时替换 `window.WebSocket`，将 `ws/wss` 目标编码到当前 Origin 的 `/__tvpn/ws`。服务端独立连接上游并逐消息双向复制，保留子协议、文本/二进制类型、32 MiB 消息上限、关闭码和关闭原因。握手 Cookie、Origin、DNS 固定和访问策略使用与 HTTP 相同的安全规则。

`document.cookie` 由运行时提供同步可见视图，写入经 `/__tvpn/cookie` 更新加密 Cookie Jar；HttpOnly Cookie 永不返回脚本。

代理页面只接受来自配置中管理界面 Origin 的 `tvpn:command` 消息，用于执行后退、前进和刷新；页面导航通过 `tvpn:navigation` 消息同步到管理界面的地址栏。
