# Tvpn

Tvpn 是一个有服务端的 WebVPN。用户登录后在浏览器式地址栏中输入目标网址，页面通过隔离的代理 Origin 加载；访问策略可按本地用户和 LDAP 组配置。

项目当前处于首版实现阶段，产品名称暂定为 Tvpn。

当前已实现 PostgreSQL 自动迁移、本地 Argon2id 账号、数据库会话与 CSRF 防护。首次启动可通过 Secret 文件创建引导管理员，详见 `docs/configuration.md`。

LDAP 登录支持 LDAPS/StartTLS、搜索绑定或 DN 模板，并在登录成功后即时创建用户、同步组成员关系；本地同名账号不会回退到 LDAP。

访问策略支持禁止内网、白名单、黑名单和禁止访问，可同时绑定用户与 LDAP 组。多个策略按交集执行，未分配策略的用户默认禁止访问。管理员 API 可维护用户、LDAP、组、策略、上游代理和最近 200 条审计事件。

要允许用户访问任意公网 HTTP/HTTPS 网站，可创建一个规则为空的“禁止内网”策略并分配给该用户；云元数据地址和 Tvpn 控制域始终不可作为代理目标。

WebVPN 转发已实现独立 Origin 路由、一次性 bootstrap、请求级策略和 DNS 重绑定防护、重定向改写，以及 AES-256-GCM 加密的服务端 Cookie Jar。管理员可以维护带可选认证的 HTTP 或 SOCKS5 上游代理并授权给用户或 LDAP 组；服务端直连也可启用独立 ACL，从而让指定用户只能使用代理出口。

注入运行时会 Hook URL 与导航行为；Fetch、XHR、SSE 和 Beacon 强制走页面级复用 WebSocket。页面创建的原生 WebSocket 使用独立隧道，已验证文本/二进制帧和子协议兼容。网址框旁提供默认关闭的兼容模式，用于需要多 Origin CORS 映射和动态资源限流的页面。具体限制见 `docs/compatibility.md`。

Web 界面提供登录、浏览器式地址栏、出口选择与内嵌代理页面。管理员可在同一界面维护本地用户、用户/LDAP 组策略、HTTP/SOCKS5 上游代理、LDAP 参数和审计日志；前端请求统一声明在 `web/src/api/client.ts`。

## 开发环境

要求 Docker 及 Docker Compose：

```sh
./scripts/bootstrap-admin.sh --build
```

本地首次运行用 `--build` 显式构建镜像、交互式创建管理员并启动容器；密码只经忽略追踪的 Docker Secret 文件传递，启动后文件会清空。已经构建或拉取镜像时直接运行 `./scripts/bootstrap-admin.sh`，脚本只重新创建应用容器，不会重新构建镜像。后续开发可运行 `./scripts/dev.sh`。本地管理地址为 `http://app.localhost:8080`，代理页面使用 `*.proxy.localhost:8080`。

从同一局域网的其他设备测试时，为 `TVPN_ACCESS_DOMAIN` 提供可通配解析到服务器的域名。例如服务器地址为 `10.96.210.226`，可使用 `TVPN_ACCESS_DOMAIN=10-96-210-226.sslip.io`，管理入口即为 `http://app.10-96-210-226.sslip.io:8080`。

公网部署可复制 `.env.example`，通过 `.env` 传入 HTTPS 管理域、代理通配域、监听地址和数据库密码。管理域可部署在代理基础域之下，例如 `vpn.proxy.example.com` 与 `proxy.example.com`，从而共用 `*.proxy.example.com` 的 DNS 和证书；Docker Compose 与 Nginx 完整示例见 `docs/deployment.md`。

默认生产示例从阿里云杭州区拉取 `fjrcn/tvpn` 和 `fjrcn/tvpn-pg`，镜像地址同样可在 `.env` 中覆盖；本地开发不配置时仍使用源码构建和官方 PostgreSQL 镜像。

## 验证

```sh
./scripts/test.sh
```

部署、环境变量、安全边界和浏览器兼容性分别见 `docs/`。本项目采用 AGPL-3.0 许可证。
