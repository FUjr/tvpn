package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

type ErrorCode string

const (
	ErrAdminRequired            ErrorCode = "admin_required"
	ErrContextNotFound          ErrorCode = "context_not_found"
	ErrCSRFFailed               ErrorCode = "csrf_failed"
	ErrDirectAccessDenied       ErrorCode = "direct_access_denied"
	ErrInternal                 ErrorCode = "internal_error"
	ErrInvalidAssignment        ErrorCode = "invalid_assignment"
	ErrInvalidCookie            ErrorCode = "invalid_cookie"
	ErrInvalidCredentials       ErrorCode = "invalid_credentials"
	ErrInvalidID                ErrorCode = "invalid_id"
	ErrInvalidLDAP              ErrorCode = "invalid_ldap"
	ErrInvalidPassword          ErrorCode = "invalid_password"
	ErrInvalidPolicy            ErrorCode = "invalid_policy"
	ErrInvalidRequest           ErrorCode = "invalid_request"
	ErrInvalidScope             ErrorCode = "invalid_scope"
	ErrInvalidTarget            ErrorCode = "invalid_target"
	ErrInvalidTicket            ErrorCode = "invalid_ticket"
	ErrInvalidToken             ErrorCode = "invalid_token"
	ErrInvalidUpstreamProxy     ErrorCode = "invalid_upstream_proxy"
	ErrInvalidUser              ErrorCode = "invalid_user"
	ErrLastAdmin                ErrorCode = "last_admin"
	ErrLDAPUnavailable          ErrorCode = "ldap_unavailable"
	ErrMethodNotAllowed         ErrorCode = "method_not_allowed"
	ErrNotFound                 ErrorCode = "not_found"
	ErrProxyAuthentication      ErrorCode = "proxy_authentication_required"
	ErrProxySessionInvalid      ErrorCode = "proxy_session_invalid"
	ErrRedirectDenied           ErrorCode = "redirect_denied"
	ErrRewriteFailed            ErrorCode = "rewrite_failed"
	ErrRouteNotFound            ErrorCode = "route_not_found"
	ErrScopeRequired            ErrorCode = "scope_required"
	ErrTargetDenied             ErrorCode = "target_denied"
	ErrTargetUnavailable        ErrorCode = "target_unavailable"
	ErrTokenNotFound            ErrorCode = "token_not_found"
	ErrUnauthorized             ErrorCode = "unauthorized"
	ErrUpstreamError            ErrorCode = "upstream_error"
	ErrUpstreamProxyDenied      ErrorCode = "upstream_proxy_denied"
	ErrUpstreamProxyNotFound    ErrorCode = "upstream_proxy_not_found"
	ErrUpstreamProxyUnavailable ErrorCode = "upstream_proxy_unavailable"
	ErrWebSocketUpstreamFailed  ErrorCode = "websocket_upstream_failed"
	ErrWebUnavailable           ErrorCode = "web_unavailable"
	ErrMisdirectedRequest       ErrorCode = "misdirected_request"
)

type errorDefinition struct {
	Status    int
	MessageID string
	ZH        string
	EN        string
}

var errorCatalog = map[ErrorCode]errorDefinition{
	ErrAdminRequired:            {403, "error.auth.admin_required", "需要管理员权限", "Administrator access is required"},
	ErrContextNotFound:          {404, "error.proxy.context_not_found", "代理上下文不存在", "Proxy context was not found"},
	ErrCSRFFailed:               {403, "error.auth.csrf_failed", "CSRF 校验失败", "CSRF validation failed"},
	ErrDirectAccessDenied:       {403, "error.proxy.direct_access_denied", "未获授权使用服务端直连", "Direct server access is not authorized"},
	ErrInternal:                 {500, "error.common.internal", "服务器内部错误", "Internal server error"},
	ErrInvalidAssignment:        {422, "error.admin.invalid_assignment", "资源授权无效", "Resource assignment is invalid"},
	ErrInvalidCookie:            {422, "error.proxy.invalid_cookie", "Cookie 格式无效", "Cookie is invalid"},
	ErrInvalidCredentials:       {401, "error.auth.invalid_credentials", "用户名或密码错误", "Invalid username or password"},
	ErrInvalidID:                {400, "error.common.invalid_id", "ID 格式无效", "ID is invalid"},
	ErrInvalidLDAP:              {422, "error.admin.invalid_ldap", "LDAP 配置无效", "LDAP configuration is invalid"},
	ErrInvalidPassword:          {422, "error.admin.invalid_password", "密码不符合要求", "Password does not meet requirements"},
	ErrInvalidPolicy:            {422, "error.admin.invalid_policy", "访问策略无效", "Access policy is invalid"},
	ErrInvalidRequest:           {400, "error.common.invalid_request", "请求格式无效", "Request is invalid"},
	ErrInvalidScope:             {422, "error.auth.invalid_scope", "令牌权限范围无效", "Token scope is invalid"},
	ErrInvalidTarget:            {422, "error.proxy.invalid_target", "目标 URL 无效", "Target URL is invalid"},
	ErrInvalidTicket:            {401, "error.proxy.invalid_ticket", "代理票据无效或已使用", "Proxy ticket is invalid or already used"},
	ErrInvalidToken:             {401, "error.auth.invalid_token", "程序令牌无效或已过期", "Program token is invalid or expired"},
	ErrInvalidUpstreamProxy:     {422, "error.admin.invalid_upstream_proxy", "上游代理配置无效", "Upstream proxy configuration is invalid"},
	ErrInvalidUser:              {422, "error.admin.invalid_user", "用户配置无效", "User configuration is invalid"},
	ErrLastAdmin:                {409, "error.admin.last_admin", "不能禁用或降级最后一个管理员", "The last administrator cannot be disabled or demoted"},
	ErrLDAPUnavailable:          {502, "error.admin.ldap_unavailable", "LDAP 连接或服务绑定失败", "LDAP connection or service bind failed"},
	ErrMethodNotAllowed:         {405, "error.common.method_not_allowed", "请求方法不受支持", "Method is not allowed"},
	ErrNotFound:                 {404, "error.common.not_found", "资源不存在", "Resource was not found"},
	ErrProxyAuthentication:      {407, "error.proxy.authentication_required", "需要有效的程序令牌或代理会话", "A valid program token or proxy session is required"},
	ErrProxySessionInvalid:      {401, "error.proxy.session_invalid", "代理会话已失效", "Proxy session has expired"},
	ErrRedirectDenied:           {403, "error.proxy.redirect_denied", "访问策略拒绝重定向目标", "Redirect target was denied by policy"},
	ErrRewriteFailed:            {502, "error.proxy.rewrite_failed", "目标页面无法安全改写", "Target page could not be rewritten safely"},
	ErrRouteNotFound:            {404, "error.proxy.route_not_found", "代理路由不存在", "Proxy route was not found"},
	ErrScopeRequired:            {403, "error.auth.scope_required", "程序令牌缺少所需权限", "Program token lacks the required scope"},
	ErrTargetDenied:             {403, "error.proxy.target_denied", "访问策略拒绝该目标", "Target was denied by policy"},
	ErrTargetUnavailable:        {502, "error.proxy.target_unavailable", "目标地址不可用", "Target is unavailable"},
	ErrTokenNotFound:            {404, "error.auth.token_not_found", "程序令牌不存在", "Program token was not found"},
	ErrUnauthorized:             {401, "error.auth.unauthorized", "需要登录或程序令牌", "Authentication is required"},
	ErrUpstreamError:            {502, "error.proxy.upstream_error", "连接目标站点失败", "Failed to connect to target"},
	ErrUpstreamProxyDenied:      {403, "error.proxy.upstream_proxy_denied", "未获授权或代理已停用", "Upstream proxy is disabled or not authorized"},
	ErrUpstreamProxyNotFound:    {404, "error.admin.upstream_proxy_not_found", "上游代理不存在", "Upstream proxy was not found"},
	ErrUpstreamProxyUnavailable: {502, "error.proxy.upstream_proxy_unavailable", "上游代理不可用或授权已撤销", "Upstream proxy is unavailable or authorization was revoked"},
	ErrWebSocketUpstreamFailed:  {502, "error.proxy.websocket_upstream_failed", "无法连接上游 WebSocket", "Failed to connect to upstream WebSocket"},
	ErrWebUnavailable:           {503, "error.common.web_unavailable", "Web 界面不可用", "Web interface is unavailable"},
	ErrMisdirectedRequest:       {421, "error.common.misdirected_request", "请求 Host 不属于此服务", "Request host does not belong to this service"},
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func Problem(w http.ResponseWriter, r *http.Request, code ErrorCode) {
	definition, ok := errorCatalog[code]
	if !ok {
		definition = errorCatalog[ErrInternal]
		code = ErrInternal
	}
	message := definition.ZH
	if prefersEnglish(r.Header.Get("Accept-Language")) {
		message = definition.EN
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Tvpn-Error-Code", string(code))
	WriteJSON(w, definition.Status, map[string]any{
		"type":       "https://tvpn.invalid/problems/" + string(code),
		"title":      http.StatusText(definition.Status),
		"status":     definition.Status,
		"code":       code,
		"message_id": definition.MessageID,
		"message":    message,
	})
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		Problem(w, r, ErrInvalidRequest)
		return false
	}
	return true
}

func prefersEnglish(value string) bool {
	for _, language := range strings.Split(strings.ToLower(value), ",") {
		language = strings.TrimSpace(strings.SplitN(language, ";", 2)[0])
		if strings.HasPrefix(language, "zh") {
			return false
		}
		if strings.HasPrefix(language, "en") {
			return true
		}
	}
	return false
}
