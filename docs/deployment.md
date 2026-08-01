# 公网部署

## 域名与 DNS

以下示例使用管理域 `vpn.example.com` 和代理基础域 `proxy.vpn.example.com`。将三个记录指向同一公网服务器：

| 记录 | 类型 | 值 |
| --- | --- | --- |
| `vpn.example.com` | A/AAAA | Tvpn 公网地址 |
| `*.proxy.vpn.example.com` | A/AAAA | Tvpn 公网地址 |

证书必须同时覆盖 `vpn.example.com` 和 `*.proxy.vpn.example.com`。通配证书需要 DNS-01 验证，HTTP-01 不能签发通配证书，参见 [Let's Encrypt Challenge Types](https://letsencrypt.org/docs/challenge-types/)。

## Compose

```sh
git clone https://github.com/FUjr/tvpn.git
cd tvpn
cp .env.example .env
```

编辑 `.env`，至少替换管理域、代理域、PostgreSQL 密码和容器 UID/GID。建议用只包含 URL 安全字符的随机十六进制数据库密码：

```sh
openssl rand -hex 32
./scripts/init-secrets.sh
docker compose up -d --build
```

`TVPN_POSTGRES_PASSWORD` 必须在 PostgreSQL 卷首次初始化前确定。已有数据卷不会因为修改 `.env` 自动更改数据库用户密码；这种情况下需要先在 PostgreSQL 内执行密码变更，再同步修改 `.env`。

生产示例把应用绑定到 `127.0.0.1:18080`，PostgreSQL 不发布宿主机端口。公网防火墙只需向 Nginx开放 TCP 80/443。

首次启动使用强密码创建管理员：

```sh
./scripts/bootstrap-admin.sh
```

引导完成后确认 `secrets/tvpn_bootstrap_admin_password` 已为空文件。不要把开发环境数据库或 `admin/admin` 测试账号迁移到公网。

## Nginx

复制 `deploy/nginx/tvpn.conf.example` 到 `/etc/nginx/conf.d/tvpn.conf` 或其他由 Nginx `http {}` 引入的位置，并将示例域名、证书路径和上游端口改成 `.env` 中的实际值。该文件是 `http` 配置片段，不是完整的顶层 `nginx.conf`。

Nginx 必须保留原始 `Host`，Tvpn 依靠 Host 区分管理域和随机代理域。WebSocket 反向代理还必须显式传递 `Upgrade` 和 `Connection`，参见 [Nginx WebSocket proxying](https://nginx.org/en/docs/http/websocket.html)。

```sh
nginx -t
systemctl reload nginx
```

验证管理域、随机代理域和证书：

```sh
curl -I https://vpn.example.com
dig +short random.proxy.vpn.example.com
openssl s_client -connect vpn.example.com:443 -servername random.proxy.vpn.example.com </dev/null
```
