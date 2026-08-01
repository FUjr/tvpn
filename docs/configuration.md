# 配置

| 环境变量 | 必需 | 说明 |
| --- | --- | --- |
| `TVPN_DATABASE_URL` | 是 | PostgreSQL 连接地址 |
| `TVPN_LISTEN_ADDRESS` | 否 | 监听地址，默认 `:8080` |
| `TVPN_APP_ORIGIN` | 是 | 管理界面完整 Origin |
| `TVPN_PROXY_BASE_DOMAIN` | 是 | 不含通配符的代理基础域名，可包含端口 |
| `TVPN_ENV` | 否 | `production` 启用生产安全检查 |
| `TVPN_BOOTSTRAP_ADMIN_USERNAME` | 否 | 仅在用户表为空时创建的本地管理员 |
| `TVPN_BOOTSTRAP_ADMIN_PASSWORD_FILE` | 条件 | 管理员密码 Secret 文件；配置引导管理员时必需 |

真实部署不得使用 Compose 示例密码。密码、LDAP 绑定凭据和主密钥将通过挂载文件或 Docker Secret 注入。

宿主机端口可在运行 Compose 时用 `TVPN_PORT` 覆盖，例如 `TVPN_PORT=18080 docker compose up -d`。

引导管理员只在数据库没有任何用户时创建。之后即使 Secret 仍然挂载，也不会覆盖账号或密码。
