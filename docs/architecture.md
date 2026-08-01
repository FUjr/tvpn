# 架构

Tvpn 是 Go 单体服务，内嵌 React 构建产物。PostgreSQL 保存身份、策略、代理上下文和审计数据。

管理界面使用 `TVPN_APP_ORIGIN`，代理内容使用 `TVPN_PROXY_BASE_DOMAIN` 下的独立 Origin。生产环境必须将二者部署在同一可注册域名下，并为代理域配置泛域名 DNS 和 TLS。每个上游 Origin 映射到不可预测的代理子域，防止不同目标站点共享浏览器同源权限。

请求路径分为：

1. 文档导航和解析前资源通过 HTTP 代理。
2. 注入运行时捕获可拦截请求并通过 WebSocket 多路复用。
3. 上游原生 WebSocket 使用独立隧道。

