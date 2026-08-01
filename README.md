# Tvpn

Tvpn 是一个有服务端的 WebVPN。用户登录后在浏览器式地址栏中输入目标网址，页面通过隔离的代理 Origin 加载；访问策略可按本地用户和 LDAP 组配置。

项目当前处于首版实现阶段，产品名称暂定为 Tvpn。

当前已实现 PostgreSQL 自动迁移、本地 Argon2id 账号、数据库会话与 CSRF 防护。首次启动可通过 Secret 文件创建引导管理员，详见 `docs/configuration.md`。

LDAP 登录支持 LDAPS/StartTLS、搜索绑定或 DN 模板，并在登录成功后即时创建用户、同步组成员关系；本地同名账号不会回退到 LDAP。

访问策略支持禁止内网、白名单、黑名单和禁止访问，可同时绑定用户与 LDAP 组。多个策略按交集执行，未分配策略的用户默认禁止访问。管理员 API 可维护用户、LDAP、组、策略和最近 200 条审计事件。

## 开发环境

要求 Docker 及 Docker Compose：

```sh
./scripts/dev.sh
```

本地管理地址为 `http://app.localhost:8080`，代理页面使用 `*.proxy.localhost:8080`。

## 验证

```sh
./scripts/test.sh
```

部署、环境变量、安全边界和浏览器兼容性分别见 `docs/`。本项目采用 AGPL-3.0 许可证。
