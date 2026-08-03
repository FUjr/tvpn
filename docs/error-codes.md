# 错误码

所有 Tvpn HTTP 错误使用 `application/problem+json`，并返回 `Tvpn-Error-Code: <code>`。该响应头由 Tvpn 生成，代理转发层会删除目标服务返回的同名头，SDK 应使用它区分 Tvpn 错误与目标服务自己的 RFC Problem。

```json
{
  "type": "https://tvpn.invalid/problems/invalid_token",
  "title": "Unauthorized",
  "status": 401,
  "code": "invalid_token",
  "message_id": "error.auth.invalid_token",
  "message": "程序令牌无效或已过期"
}
```

程序必须依据 `code` 或 `message_id` 分支，不能解析 `message`。服务端根据 `Accept-Language` 返回中文或英文 `message`；目前支持 `zh` 和 `en`，默认中文。

| HTTP | code | message_id |
| ---: | --- | --- |
| 400 | `invalid_id` | `error.common.invalid_id` |
| 400 | `invalid_request` | `error.common.invalid_request` |
| 401 | `invalid_credentials` | `error.auth.invalid_credentials` |
| 401 | `invalid_ticket` | `error.proxy.invalid_ticket` |
| 401 | `invalid_token` | `error.auth.invalid_token` |
| 401 | `proxy_session_invalid` | `error.proxy.session_invalid` |
| 401 | `unauthorized` | `error.auth.unauthorized` |
| 403 | `admin_required` | `error.auth.admin_required` |
| 403 | `csrf_failed` | `error.auth.csrf_failed` |
| 403 | `direct_access_denied` | `error.proxy.direct_access_denied` |
| 403 | `redirect_denied` | `error.proxy.redirect_denied` |
| 403 | `scope_required` | `error.auth.scope_required` |
| 403 | `target_denied` | `error.proxy.target_denied` |
| 403 | `upstream_proxy_denied` | `error.proxy.upstream_proxy_denied` |
| 404 | `context_not_found` | `error.proxy.context_not_found` |
| 404 | `not_found` | `error.common.not_found` |
| 404 | `route_not_found` | `error.proxy.route_not_found` |
| 404 | `token_not_found` | `error.auth.token_not_found` |
| 404 | `upstream_proxy_not_found` | `error.admin.upstream_proxy_not_found` |
| 405 | `method_not_allowed` | `error.common.method_not_allowed` |
| 409 | `last_admin` | `error.admin.last_admin` |
| 421 | `misdirected_request` | `error.common.misdirected_request` |
| 422 | `invalid_assignment` | `error.admin.invalid_assignment` |
| 422 | `invalid_cookie` | `error.proxy.invalid_cookie` |
| 422 | `invalid_ldap` | `error.admin.invalid_ldap` |
| 422 | `invalid_password` | `error.admin.invalid_password` |
| 422 | `invalid_policy` | `error.admin.invalid_policy` |
| 422 | `invalid_scope` | `error.auth.invalid_scope` |
| 422 | `invalid_target` | `error.proxy.invalid_target` |
| 422 | `invalid_upstream_proxy` | `error.admin.invalid_upstream_proxy` |
| 422 | `invalid_user` | `error.admin.invalid_user` |
| 407 | `proxy_authentication_required` | `error.proxy.authentication_required` |
| 500 | `internal_error` | `error.common.internal` |
| 502 | `ldap_unavailable` | `error.admin.ldap_unavailable` |
| 502 | `rewrite_failed` | `error.proxy.rewrite_failed` |
| 502 | `target_unavailable` | `error.proxy.target_unavailable` |
| 502 | `upstream_error` | `error.proxy.upstream_error` |
| 502 | `upstream_proxy_unavailable` | `error.proxy.upstream_proxy_unavailable` |
| 502 | `websocket_upstream_failed` | `error.proxy.websocket_upstream_failed` |
| 503 | `web_unavailable` | `error.common.web_unavailable` |

`407` 响应同时返回 `Proxy-Authenticate: Bearer realm="tvpn"`。

## Mux 通道错误

`/__tvpn/mux` 的 `response/error` 帧和协议关闭原因只返回稳定 `message_id`：

| message_id | 含义 |
| --- | --- |
| `error.proxy.mux.invalid_frame` | 帧版本、类型或长度无效 |
| `error.proxy.mux.duplicate_stream` | 流 ID 已存在 |
| `error.proxy.mux.invalid_metadata` | 请求元数据无法解析 |
| `error.proxy.mux.body_too_large` | 请求正文超过 32 MiB |
| `error.proxy.mux.unsupported_method` | 请求方法不受支持 |
| `error.proxy.mux.response_interrupted` | 上游响应传输中断 |
| `error.proxy.target_denied` | 策略拒绝目标 |
| `error.proxy.upstream_error` | 上游请求失败 |
