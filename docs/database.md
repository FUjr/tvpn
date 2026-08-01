# 数据库

迁移文件嵌入 Go 二进制并在启动时执行。部署时只允许一个实例执行数据库升级；每个迁移同时提供 Up 和 Down，CI 验证全新数据库及升级路径。

## 001 身份认证

- `users`：本地或 LDAP 用户、规范化用户名、管理员和禁用状态。本地用户必须有 Argon2id 密码摘要。
- `sessions`：仅保存会话令牌 SHA-256 摘要、CSRF Token、过期时间和最后访问时间；删除用户会级联删除会话。

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
