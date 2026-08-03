# 安全边界

## Origin 隔离

管理界面、代理基础域和每个上游 Origin 使用不同浏览器 Origin。应用会话是管理域 Host-only Cookie；代理会话只对代理基础域有效。一次性票据负责在二者间建立关联，不把应用会话交给上游脚本。

程序调用使用可撤销的 PAT。管理 API 从 `Authorization` 读取 PAT；随机代理域从 `Proxy-Authorization` 读取同一 PAT并在转发前删除该头，因此目标接口可以独立使用自己的 `Authorization`。PAT 只保存 SHA-256 摘要，并同时校验过期、撤销、Scope、所属用户状态和当前管理员状态。Bearer 鉴权不依赖浏览器 Cookie，因而不执行 CSRF；令牌创建和撤销只允许浏览器会话并继续要求 CSRF。

## SSRF 与策略

- 用户入口可省略协议并自动补全为 `http://`；进入策略与代理层前只接受无用户信息的绝对 HTTP/HTTPS URL，显式 `https://` 保持不变。
- URL 主机经过 IDNA 规范化，端口必须在 1-65535。
- 每次请求解析全部 A/AAAA 地址，策略通过后固定其中一个地址拨号，TLS ServerName 与 HTTP Host 仍使用原始主机。HTTP 上游代理通过 CONNECT 连接固定 IP，SOCKS5 也只接收固定 IP，不能在代理端重新解析目标域名。
- 云元数据地址、管理域和代理域无条件拒绝；其余私网访问由用户与 LDAP 组策略决定。
- 重定向、Cookie、后续资源和 WebSocket 不能绕过相同校验。

## Cookie 与日志

上游 Cookie 值和上游代理密码使用 AES-256-GCM 及各自关联数据加密。代理密码不通过读取 API 返回，更新时留空会保留原值。Domain Cookie 必须属于响应主机且不能是公共后缀。审计 URL 删除查询参数和 Fragment，不记录正文、认证头、代理密码或 Cookie。

代理授权在每次连接时重新确认。服务端直连可启用同样的用户/LDAP 组 ACL，服务端在创建直连上下文和实际拨号时都检查授权，不能通过直接构造 API 请求绕过。代理被停用或出口授权被撤销后，已有浏览上下文不能继续通过它建立新连接；切换出口会关闭旧上下文，避免 Cookie Jar 和出口在同一上下文内混用。已经建立的 WebSocket 不会被主动断开。
