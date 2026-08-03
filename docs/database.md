# 数据库

迁移文件嵌入 Go 二进制并在启动时执行。部署时只允许一个实例执行数据库升级；每个迁移同时提供 Up 和 Down，CI 验证全新数据库及升级路径。

## 001 身份认证

- `users`：本地或 LDAP 用户、规范化用户名、管理员和禁用状态。本地用户必须有 Argon2id 密码摘要。
- `sessions`：仅保存会话令牌 SHA-256 摘要、CSRF Token、过期时间和最后访问时间；删除用户会级联删除会话。
- `program_tokens`：保存 PAT 的 SHA-256 摘要、不可用于认证的显示前缀、`proxy/admin` Scope、过期/撤销/最近使用时间；明文令牌只在创建时返回一次，删除用户会级联删除令牌。

用户名去除首尾空白并按小写唯一比较，展示时保留原始大小写。

## 002 LDAP

- `ldap_settings`：单行保存非敏感连接、搜索、属性和组映射设置；绑定密码与 CA 不进入数据库。
- `ldap_groups`：按外部 DN 唯一保存登录过程中发现的组。
- `user_ldap_groups`：每次 LDAP 登录成功后在事务中全量替换用户的当前组成员关系。

## 003 策略与审计

- `policies`、`policy_rules`：保存四种策略及精确主机、域名后缀、CIDR、URL 前缀规则。
- `user_policies`、`ldap_group_policies`：分别绑定用户和 LDAP 组；请求必须通过全部有效策略。
- `audit_events`：保存认证、配置、顶层导航和拒绝事件，不保存请求正文、Cookie 或 URL 查询参数。

## 004 代理上下文

- `proxy_contexts`、`proxy_routes`：将用户浏览上下文及每个上游 Origin 映射到随机代理子域。
- `proxy_tickets`、`proxy_sessions`：保存一次性票据和代理域会话的令牌摘要，管理域 Cookie 不会发送给代理页面。
- `proxy_cookies`：按原始 Domain、Path、Secure 等属性保存上游 Cookie；值使用 `TVPN_MASTER_KEY_FILE` 加密。

## 005 上游代理

- `upstream_proxies`：保存 HTTP/SOCKS5 代理地址、状态和可选用户名；密码使用 `TVPN_MASTER_KEY_FILE` 加密，接口只返回是否已配置。
- `user_upstream_proxies`、`ldap_group_upstream_proxies`：分别保存用户和 LDAP 组授权，用户有效列表取两者并集。
- `proxy_contexts.upstream_proxy_id`：固定该浏览上下文选择的出口；空值表示服务端直连。删除代理会同时关闭并删除引用它的上下文。

## 006 服务端直连 ACL

- `direct_access_settings`：单行保存直连是否启用 ACL；默认关闭 ACL，升级后继续允许全部用户直连。
- `direct_access_users`、`direct_access_ldap_groups`：保存直连的用户和 LDAP 组授权。启用 ACL 后用户满足任一授权即可使用直连，空授权表示无人可直连。

## 007 代理兼容模式

- `proxy_contexts.compatibility_mode`：保存该浏览上下文是否启用额外跨域和动态资源兼容处理；默认关闭，避免改变已有上下文行为。
