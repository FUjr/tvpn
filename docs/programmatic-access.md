# 程序调用与 SDK

Tvpn 程序调用使用可撤销的 Personal Access Token（PAT）。令牌以 `tvpn_pat_` 开头，只在创建响应中返回一次；数据库只保存 SHA-256 摘要、可识别前缀、权限、过期时间和最近使用时间。

## 鉴权协议

管理 API 使用：

```http
Authorization: Bearer tvpn_pat_...
```

随机代理域使用：

```http
Proxy-Authorization: Bearer tvpn_pat_...
```

`Proxy-Authorization` 在 Tvpn 完成鉴权后会被删除，不会发送给目标服务。目标服务自己的 `Authorization` 保持不变，因此两套 Bearer Token 可以同时使用。PAT 请求不需要 CSRF；浏览器 Cookie 会话仍要求 CSRF。

令牌权限：

| Scope | 用途 |
| --- | --- |
| `proxy` | 查询出口、创建/导航/关闭代理上下文，并访问该用户拥有的随机代理路由 |
| `admin` | 调用管理员 API；同时要求令牌所属用户当前仍是管理员 |

用户在 Web 界面的“程序令牌”页创建、查看元数据和撤销令牌。默认过期时间为 90 天，最长 366 天。禁用用户、令牌过期或撤销会立即阻止新请求。

## 原始 HTTP 流程

1. 使用 `Authorization: Bearer` 调用 `POST /api/v1/proxy/contexts/`。
2. 请求体传入目标绝对 URL、可选 `upstream_proxy_id` 和 `compatibility_mode`。
3. 读取响应中的 `context.id` 和 `route_url`；程序调用不需要访问 `bootstrap_url`。
4. 使用 `Proxy-Authorization: Bearer` 请求 `route_url`。目标接口的请求方法、正文和业务头会正常转发。
5. 使用 `Authorization: Bearer` 调用 `DELETE /api/v1/proxy/contexts/{id}`。

每个上下文拥有独立、服务端加密的目标 Cookie Jar，并固定使用创建时选择的出口。SDK 的单次 `request` 会自动完成上述创建和清理流程。

## CLI

```sh
go install github.com/FUjr/tvpn/cmd/tvpnctl@latest

export TVPN_SERVER=https://vpn.proxy.example.com
export TVPN_TOKEN=tvpn_pat_REDACTED

tvpnctl request GET https://api.example.com/v1/status
tvpnctl request \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer $TARGET_TOKEN" \
  --data @request.json \
  POST https://api.example.com/v1/jobs
```

令牌应通过环境变量或 Secret 管理器注入，不要写入命令历史、源码或配置仓库。

## Go SDK

```go
client := tvpn.NewClient(os.Getenv("TVPN_SERVER"), os.Getenv("TVPN_TOKEN"))
response, err := client.Do(ctx, tvpn.Request{
    Method: http.MethodGet,
    URL:    "https://api.example.com/v1/devices",
    Header: http.Header{"Authorization": {"Bearer " + targetToken}},
})
if err != nil { /* inspect *tvpn.Problem */ }
defer response.Body.Close()
```

Go 响应正文关闭时自动关闭代理上下文，因此调用方必须关闭 `response.Body`。

需要复用目标 Cookie 登录态时使用持久 Session：

```go
session, err := client.Open(ctx, "https://api.example.com/login", tvpn.SessionOptions{})
if err != nil { /* ... */ }
defer session.Close(ctx)

login, err := session.Do(ctx, tvpn.Request{Method: http.MethodPost, URL: "https://api.example.com/login", Body: loginBody})
if err == nil { login.Body.Close() }
devices, err := session.Do(ctx, tvpn.Request{Method: http.MethodGet, URL: "https://api.example.com/v1/devices"})
```

## Python SDK

```sh
python -m pip install ./sdk/python
```

```python
from tvpn import Client, Problem

client = Client(os.environ["TVPN_SERVER"], os.environ["TVPN_TOKEN"])
response = client.get(
    "https://api.example.com/v1/devices",
    headers={"Authorization": f"Bearer {target_token}"},
)
print(response.status, response.json())
```

Python SDK 只使用标准库，响应会在内存中缓冲后关闭上下文。

```python
with client.session("https://api.example.com/login") as session:
    session.post("https://api.example.com/login", json_body={"username": "...", "password": "..."})
    devices = session.get("https://api.example.com/v1/devices")
```

同一 Session 中的请求共享目标 Cookie Jar 和出口；凭据只发送给目标接口，不由 SDK 保存。

## TypeScript SDK

```sh
npm install ./sdk/typescript
```

```ts
import { TvpnClient, TvpnProblem } from '@tvpn/sdk'

const client = new TvpnClient({
  baseURL: process.env.TVPN_SERVER!,
  token: process.env.TVPN_TOKEN!,
})
const response = await client.fetch('https://api.example.com/v1/devices', {
  headers: { Authorization: `Bearer ${targetToken}` },
})
console.log(response.status, await response.json())
```

TypeScript SDK 面向提供标准 Fetch API 的 Node.js 18+ 运行时。响应会缓冲后关闭上下文。

```ts
const session = await client.open('https://api.example.com/login')
try {
  await session.fetch('https://api.example.com/login', { method: 'POST', json: { username: '...', password: '...' } })
  const devices = await session.fetch('https://api.example.com/v1/devices')
} finally {
  await session.close()
}
```

## 错误处理

Tvpn 自身错误使用 `application/problem+json` 并带有受保护的 `Tvpn-Error-Code` 响应头。转发层会删除目标接口返回的同名头，SDK 据此区分 Tvpn 错误与目标接口自己的 4xx/5xx，包括目标接口自己的 RFC Problem：Tvpn 错误抛出结构化 `Problem`，目标接口响应保持普通响应。完整错误码见 [error-codes.md](error-codes.md)。
