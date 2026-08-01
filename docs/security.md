# 安全边界

## Origin 隔离

管理界面、代理基础域和每个上游 Origin 使用不同浏览器 Origin。应用会话是管理域 Host-only Cookie；代理会话只对代理基础域有效。一次性票据负责在二者间建立关联，不把应用会话交给上游脚本。

## SSRF 与策略

- 只接受无用户信息的绝对 HTTP/HTTPS URL。
- URL 主机经过 IDNA 规范化，端口必须在 1-65535。
- 每次请求解析全部 A/AAAA 地址，策略通过后固定其中一个地址拨号，TLS ServerName 仍使用原始主机。
- 云元数据地址、管理域和代理域无条件拒绝；其余私网访问由用户与 LDAP 组策略决定。
- 重定向、Cookie、后续资源和 WebSocket 不能绕过相同校验。

## Cookie 与日志

上游 Cookie 值使用 AES-256-GCM 和上下文关联数据加密。Domain Cookie 必须属于响应主机且不能是公共后缀。审计 URL 删除查询参数和 Fragment，不记录正文、认证头或 Cookie。

