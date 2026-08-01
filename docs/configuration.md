# 配置

| 环境变量 | 必需 | 说明 |
| --- | --- | --- |
| `TVPN_DATABASE_URL` | 是 | PostgreSQL 连接地址 |
| `TVPN_LISTEN_ADDRESS` | 否 | 监听地址，默认 `:8080` |
| `TVPN_APP_ORIGIN` | 是 | 管理界面完整 Origin |
| `TVPN_PROXY_BASE_DOMAIN` | 是 | 不含通配符的代理基础域名，可包含端口 |
| `TVPN_ENV` | 否 | `production` 启用生产安全检查 |

真实部署不得使用 Compose 示例密码。密码、LDAP 绑定凭据和主密钥将通过挂载文件或 Docker Secret 注入。

