# Tvpn

Tvpn 是一个有服务端的 WebVPN。用户登录后在浏览器式地址栏中输入目标网址，页面通过隔离的代理 Origin 加载；访问策略可按本地用户和 LDAP 组配置。

项目当前处于首版实现阶段，产品名称暂定为 Tvpn。

当前已实现 PostgreSQL 自动迁移、本地 Argon2id 账号、数据库会话与 CSRF 防护。首次启动可通过 Secret 文件创建引导管理员，详见 `docs/configuration.md`。

LDAP 登录支持 LDAPS/StartTLS、搜索绑定或 DN 模板，并在登录成功后即时创建用户、同步组成员关系；本地同名账号不会回退到 LDAP。

访问策略支持禁止内网、白名单、黑名单和禁止访问，可同时绑定用户与 LDAP 组。多个策略按交集执行，未分配策略的用户默认禁止访问。管理员 API 可维护用户、LDAP、组、策略和最近 200 条审计事件。

HTTP 代理已实现独立 Origin 路由、一次性 bootstrap、请求级策略和 DNS 重绑定防护、重定向改写，以及 AES-256-GCM 加密的服务端 Cookie Jar。

注入运行时会 Hook URL 与导航行为；Fetch、XHR、SSE 和 Beacon 强制走页面级复用 WebSocket。页面创建的原生 WebSocket 使用独立隧道，已验证文本/二进制帧和子协议兼容。具体限制见 `docs/compatibility.md`。

Web 界面提供登录、浏览器式地址栏与内嵌代理页面。管理员可在同一界面维护本地用户、用户/LDAP 组策略、LDAP 参数和审计日志；前端请求统一声明在 `web/src/api/client.ts`。

## 开发环境

要求 Docker 及 Docker Compose：

```sh
./scripts/bootstrap-admin.sh
```

首次运行用该脚本交互式创建管理员并启动容器；密码只经忽略追踪的 Docker Secret 文件传递，启动后文件会清空。后续直接运行 `./scripts/dev.sh`。本地管理地址为 `http://app.localhost:8080`，代理页面使用 `*.proxy.localhost:8080`。

## 验证

```sh
./scripts/test.sh
```

部署、环境变量、安全边界和浏览器兼容性分别见 `docs/`。本项目采用 AGPL-3.0 许可证。
