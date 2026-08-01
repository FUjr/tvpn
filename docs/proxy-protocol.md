# 代理协议

## HTTP 路由

- `bootstrap.<proxy-base>/__tvpn/bootstrap?ticket=...`：消费一次性票据并设置代理会话。
- `<route-id>.<proxy-base>/<path>`：`route-id` 映射到一个用户上下文和原始 HTTP/HTTPS Origin。

服务端保留请求方法和流式正文，删除 hop-by-hop 与 `X-Forwarded-*` 头，重建上游 Host、Origin、Referer 和 Cookie。响应侧保存并移除上游 `Set-Cookie`，重写 `Location`，删除阻止嵌入的 frame/CSP 响应头，并流式返回正文。

## WebSocket

浏览器运行时及 `/__tvpn/mux`、`/__tvpn/ws` 的帧协议将在 WebSocket 兼容提交中定义。原生 WebSocket 与普通请求复用通道分离，以保留子协议和消息边界。

