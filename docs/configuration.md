# 配置

| 环境变量 | 必需 | 说明 |
| --- | --- | --- |
| `TVPN_DATABASE_URL` | 是 | PostgreSQL 连接地址 |
| `TVPN_LISTEN_ADDRESS` | 否 | 监听地址，默认 `:8080` |
| `TVPN_APP_ORIGIN` | 是 | 管理界面完整 Origin；管理域可以是代理基础域的直接子域 |
| `TVPN_PROXY_BASE_DOMAIN` | 是 | 不含协议和通配符的代理基础域名，可包含端口 |
| `TVPN_ENV` | 否 | `production` 启用生产安全检查 |
| `TVPN_BOOTSTRAP_ADMIN_USERNAME` | 否 | 仅在用户表为空时创建的本地管理员 |
| `TVPN_BOOTSTRAP_ADMIN_PASSWORD_FILE` | 条件 | 管理员密码 Secret 文件；配置引导管理员时必需 |
| `TVPN_LDAP_BIND_PASSWORD_FILE` | 条件 | LDAP 搜索账号的绑定密码文件 |
| `TVPN_LDAP_CA_FILE` | 否 | 私有 LDAP CA 的 PEM 文件 |
| `TVPN_LDAP_ALLOW_INSECURE` | 否 | 仅开发环境允许明文 LDAP，生产环境拒绝启动 |
| `TVPN_MASTER_KEY_FILE` | 生产必需 | 恰好 32 字节的代理 Cookie 与上游代理密码加密主密钥文件 |

Compose 额外接受 `TVPN_ACCESS_DOMAIN`，默认 `localhost`。该值用于组合管理域 `app.<domain>` 和代理通配域 `*.proxy.<domain>`；从其他设备访问时必须让这两个域名都解析到 Tvpn 服务器，不能直接使用 `localhost`。

Compose 会优先读取 `.env` 中的显式配置：`TVPN_APP_ORIGIN`、`TVPN_PROXY_BASE_DOMAIN`、`TVPN_ENV`、`TVPN_BIND_ADDRESS`、`TVPN_PORT`、`TVPN_POSTGRES_PASSWORD` 和可选的 `TVPN_DATABASE_URL`。公网部署应从 `.env.example` 创建 `.env`，其中管理 Origin 使用 HTTPS，代理基础域不包含协议或通配符。

支持让管理域与随机代理域共用一个通配证书和通配 DNS 记录，例如：

```dotenv
TVPN_APP_ORIGIN=https://vpn.proxy.example.com
TVPN_PROXY_BASE_DOMAIN=proxy.example.com
```

此时管理入口精确匹配 `vpn.proxy.example.com`，`bootstrap.proxy.example.com` 和随机路由域仍由代理处理。不要将两项配置成完全相同的主机名。

真实部署不得使用 Compose 示例密码。密码、LDAP 绑定凭据和主密钥将通过挂载文件或 Docker Secret 注入。

宿主机端口可在运行 Compose 时用 `TVPN_PORT` 覆盖，例如 `TVPN_PORT=18080 docker compose up -d`。

局域网临时测试可使用能按地址自动解析的通配 DNS，例如：

```sh
TVPN_PORT=18888 TVPN_ACCESS_DOMAIN=10-96-210-226.sslip.io ./scripts/dev.sh
```

正式部署应改用自有域名、泛域名 DNS 和 HTTPS，不应依赖公共通配 DNS 服务。

首次启动运行 `scripts/bootstrap-admin.sh`，交互输入的管理员密码经 `secrets/tvpn_bootstrap_admin_password` 传给容器，账号确认创建后该文件立即清空。脚本默认使用现有镜像，只重新创建应用容器以读取一次性引导配置；本地没有应用镜像时可显式运行 `scripts/bootstrap-admin.sh --build`。脚本和环境变量只在用户表为空时生效，不会覆盖现有管理员。

`scripts/dev.sh` 会在首次运行时生成忽略版本控制的 `secrets/tvpn_master_key` 和空的引导密码 Secret。直接运行 Compose 前应先执行 `scripts/init-secrets.sh`；更换主密钥会使现有上游 Cookie 和已保存的上游代理密码无法解密。

本地 Compose 通过 `TVPN_CONTAINER_UID`、`TVPN_CONTAINER_GID` 让非 root 容器读取宿主机 0600 Secret，`scripts/dev.sh` 会自动传入当前用户 ID。生产环境应使用容器平台的 Secret 权限映射。

引导管理员只在数据库没有任何用户时创建。之后即使 Secret 仍然挂载，也不会覆盖账号或密码。

LDAP 支持 `ldaps://` 或 `ldap://` 加 StartTLS。搜索绑定模式先用服务账号定位唯一用户，再使用用户密码绑定；DN 模板模式适合目录结构固定的部署。LDAP 密码只用于当前绑定，不保存到数据库或日志。
