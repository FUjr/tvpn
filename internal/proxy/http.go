package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FUjr/tvpn/internal/auth"
	"github.com/FUjr/tvpn/internal/httpapi"
	"github.com/FUjr/tvpn/internal/policy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const proxyCookieName = "tvpn_proxy_session"

type Service struct {
	store           *Store
	policyStore     *policy.Store
	authStore       *auth.Store
	guard           *Guard
	cipher          *Cipher
	appOrigin       *url.URL
	proxyBaseDomain string
	secure          bool
	sessionTTL      time.Duration
}

func NewService(db *pgxpool.Pool, authStore *auth.Store, appOrigin, proxyBaseDomain string, key []byte, secure bool, sessionTTL time.Duration) (*Service, error) {
	guard, err := NewGuard(appOrigin, proxyBaseDomain)
	if err != nil {
		return nil, err
	}
	cipher, err := NewCipher(key)
	if err != nil {
		return nil, err
	}
	app, err := url.Parse(appOrigin)
	if err != nil {
		return nil, err
	}
	return &Service{store: NewStore(db), policyStore: policy.NewStore(db), authStore: authStore, guard: guard, cipher: cipher, appOrigin: app, proxyBaseDomain: proxyBaseDomain, secure: secure, sessionTTL: sessionTTL}, nil
}

func (s *Service) AppRoutes() http.Handler {
	r := chi.NewRouter()
	r.Post("/", s.createContext)
	r.Get("/{id}", s.getContext)
	r.Post("/{id}/navigate", s.navigate)
	r.Delete("/{id}", s.closeContext)
	return r
}

func (s *Service) createContext(w http.ResponseWriter, r *http.Request) {
	var input struct {
		URL string `json:"url"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	session, _ := auth.SessionFromContext(r.Context())
	target, decision, err := s.authorize(r.Context(), session.User.ID, input.URL)
	if err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_target", err.Error())
		return
	}
	if !decision.Allowed {
		s.audit(r.Context(), session.User.ID, "proxy.denied", input.URL, "denied")
		httpapi.Problem(w, http.StatusForbidden, "target_denied", "访问策略拒绝该目标")
		return
	}
	contextValue, route, ticket, err := s.store.CreateContext(r.Context(), session.User.ID, target.URL)
	if err != nil {
		proxyInternal(w)
		return
	}
	s.audit(r.Context(), session.User.ID, "proxy.context.create", target.URL.String(), "success")
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"context": contextValue, "bootstrap_url": s.bootstrapURL(ticket), "route_url": s.routeURL(route, target.URL)})
}
func (s *Service) getContext(w http.ResponseWriter, r *http.Request) {
	id, ok := proxyID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	session, _ := auth.SessionFromContext(r.Context())
	value, err := s.store.Context(r.Context(), id, session.User.ID)
	if err != nil {
		httpapi.Problem(w, http.StatusNotFound, "context_not_found", "代理上下文不存在")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, value)
}
func (s *Service) navigate(w http.ResponseWriter, r *http.Request) {
	id, ok := proxyID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input struct {
		URL string `json:"url"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	session, _ := auth.SessionFromContext(r.Context())
	if _, err := s.store.Context(r.Context(), id, session.User.ID); err != nil {
		httpapi.Problem(w, http.StatusNotFound, "context_not_found", "代理上下文不存在")
		return
	}
	target, decision, err := s.authorize(r.Context(), session.User.ID, input.URL)
	if err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_target", err.Error())
		return
	}
	if !decision.Allowed {
		s.audit(r.Context(), session.User.ID, "proxy.denied", input.URL, "denied")
		httpapi.Problem(w, http.StatusForbidden, "target_denied", "访问策略拒绝该目标")
		return
	}
	route, err := s.store.ResolveRoute(r.Context(), id, session.User.ID, target.URL)
	if err != nil {
		proxyInternal(w)
		return
	}
	s.store.UpdateCurrentURL(r.Context(), id, target.URL.String())
	ticket, err := s.store.CreateTicket(r.Context(), session.User.ID, id)
	if err != nil {
		proxyInternal(w)
		return
	}
	s.audit(r.Context(), session.User.ID, "proxy.navigate", target.URL.String(), "allowed")
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"bootstrap_url": s.bootstrapURL(ticket), "route_url": s.routeURL(route, target.URL)})
}
func (s *Service) closeContext(w http.ResponseWriter, r *http.Request) {
	id, ok := proxyID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	session, _ := auth.SessionFromContext(r.Context())
	if err := s.store.CloseContext(r.Context(), id, session.User.ID); err != nil {
		httpapi.Problem(w, http.StatusNotFound, "context_not_found", "代理上下文不存在")
		return
	}
	s.audit(r.Context(), session.User.ID, "proxy.context.close", id.String(), "success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(hostname(r.Host))
	base := strings.ToLower(hostname(s.proxyBaseDomain))
	if host == "bootstrap."+base {
		s.bootstrap(w, r)
		return
	}
	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		http.NotFound(w, r)
		return
	}
	label := strings.TrimSuffix(host, suffix)
	if strings.Contains(label, ".") {
		http.NotFound(w, r)
		return
	}
	route, err := s.store.RouteByLabel(r.Context(), label)
	if err != nil {
		httpapi.Problem(w, http.StatusNotFound, "route_not_found", "代理路由不存在")
		return
	}
	cookie, err := r.Cookie(proxyCookieName)
	if err != nil {
		httpapi.Problem(w, http.StatusUnauthorized, "proxy_session_required", "代理会话不存在")
		return
	}
	userID, err := s.store.AuthenticateProxy(r.Context(), cookie.Value)
	if err != nil || userID != route.UserID {
		httpapi.Problem(w, http.StatusUnauthorized, "proxy_session_invalid", "代理会话已失效")
		return
	}
	switch r.URL.Path {
	case "/__tvpn/runtime.js":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(runtimeJS)
		return
	case "/__tvpn/resolve":
		s.resolveURL(w, r, route)
		return
	case "/__tvpn/cookie":
		s.setDocumentCookie(w, r, route)
		return
	case "/__tvpn/mux":
		s.serveMux(w, r, route)
		return
	case "/__tvpn/ws":
		s.serveWebSocket(w, r, route)
		return
	}
	s.forward(w, r, route)
}

func (s *Service) setDocumentCookie(w http.ResponseWriter, r *http.Request, route Route) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Cookie string `json:"cookie"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	cookie, err := http.ParseSetCookie(input.Cookie)
	if err != nil || cookie.HttpOnly {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_cookie", "Cookie 格式无效")
		return
	}
	cookie.HttpOnly = false
	target, err := url.Parse(route.Scheme + "://" + hostWithPort(route.Host, route.Port, route.Scheme) + r.Header.Get("X-Tvpn-Upstream-Path"))
	if err != nil {
		proxyInternal(w)
		return
	}
	if err := s.store.SaveCookies(r.Context(), s.cipher, route.ContextID, target, []*http.Cookie{cookie}); err != nil {
		proxyInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) resolveURL(w http.ResponseWriter, r *http.Request, route Route) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		URL string `json:"url"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	input.URL = s.normalizeClientURL(r.Context(), route, input.URL)
	target, decision, err := s.authorize(r.Context(), route.UserID, input.URL)
	if err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_target", "目标 URL 无效")
		return
	}
	if !decision.Allowed {
		s.audit(r.Context(), route.UserID, "proxy.denied", input.URL, "denied")
		httpapi.Problem(w, http.StatusForbidden, "target_denied", "访问策略拒绝该目标")
		return
	}
	mapped, err := s.store.ResolveRoute(r.Context(), route.ContextID, route.UserID, target.URL)
	if err != nil {
		proxyInternal(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"url": s.routeURL(mapped, target.URL)})
}

func (s *Service) bootstrap(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("ticket")
	sessionToken, userID, contextID, err := s.store.ConsumeTicket(r.Context(), token, s.sessionTTL)
	if err != nil {
		httpapi.Problem(w, http.StatusUnauthorized, "invalid_ticket", "代理票据无效或已使用")
		return
	}
	contextValue, err := s.store.Context(r.Context(), contextID, userID)
	if err != nil {
		proxyInternal(w)
		return
	}
	target, err := s.guard.Resolve(r.Context(), contextValue.CurrentURL)
	if err != nil {
		proxyInternal(w)
		return
	}
	route, err := s.store.ResolveRoute(r.Context(), contextID, userID, target.URL)
	if err != nil {
		proxyInternal(w)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: proxyCookieName, Value: sessionToken, Path: "/", Domain: "." + hostname(s.proxyBaseDomain), Expires: time.Now().Add(s.sessionTTL), MaxAge: int(s.sessionTTL.Seconds()), HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, s.routeURL(route, target.URL), http.StatusSeeOther)
}

func (s *Service) forward(w http.ResponseWriter, r *http.Request, route Route) {
	raw := route.Scheme + "://" + hostWithPort(route.Host, route.Port, route.Scheme) + r.URL.EscapedPath()
	if r.URL.RawQuery != "" {
		raw += "?" + r.URL.RawQuery
	}
	target, decision, err := s.authorize(r.Context(), route.UserID, raw)
	if err != nil {
		httpapi.Problem(w, http.StatusBadGateway, "target_unavailable", "目标地址不可用")
		return
	}
	if !decision.Allowed {
		s.audit(r.Context(), route.UserID, "proxy.denied", target.URL.String(), "denied")
		httpapi.Problem(w, http.StatusForbidden, "target_denied", "访问策略拒绝该请求")
		return
	}
	out := r.Clone(r.Context())
	out.URL = target.URL
	out.RequestURI = ""
	out.Host = target.URL.Host
	out.Header = r.Header.Clone()
	stripHopHeaders(out.Header)
	out.Header.Del("Cookie")
	out.Header.Del("Accept-Encoding")
	out.Header.Del("X-Forwarded-For")
	out.Header.Del("X-Forwarded-Host")
	out.Header.Del("X-Forwarded-Proto")
	browserOrigin := out.Header.Get("Origin")
	upstreamOrigin := ""
	if browserOrigin != "" {
		if mapped, ok := s.mapProxyURL(r.Context(), browserOrigin); ok {
			upstreamOrigin = mapped
			out.Header.Set("Origin", mapped)
		} else {
			out.Header.Del("Origin")
		}
	}
	if referer := out.Header.Get("Referer"); referer != "" {
		if mapped, ok := s.mapProxyURL(r.Context(), referer); ok {
			out.Header.Set("Referer", mapped)
		} else {
			out.Header.Del("Referer")
		}
	}
	cookies, err := s.store.Cookies(r.Context(), s.cipher, route.ContextID, target.URL)
	if err != nil {
		proxyInternal(w)
		return
	}
	for _, cookie := range cookies {
		out.AddCookie(cookie)
	}
	transport := s.transport(target)
	response, err := transport.RoundTrip(out)
	if err != nil {
		httpapi.Problem(w, http.StatusBadGateway, "upstream_error", "连接目标站点失败")
		return
	}
	defer response.Body.Close()
	defer transport.CloseIdleConnections()
	if err := s.store.SaveCookies(r.Context(), s.cipher, route.ContextID, target.URL, response.Cookies()); err != nil {
		proxyInternal(w)
		return
	}
	if location := response.Header.Get("Location"); location != "" {
		resolved, err := target.URL.Parse(location)
		if err == nil {
			redirect, redirectDecision, resolveErr := s.authorize(r.Context(), route.UserID, resolved.String())
			if resolveErr != nil || !redirectDecision.Allowed {
				httpapi.Problem(w, http.StatusForbidden, "redirect_denied", "访问策略拒绝重定向目标")
				return
			}
			redirectRoute, routeErr := s.store.ResolveRoute(r.Context(), route.ContextID, route.UserID, redirect.URL)
			if routeErr != nil {
				proxyInternal(w)
				return
			}
			response.Header.Set("Location", s.routeURL(redirectRoute, redirect.URL))
		}
	}
	if browserOrigin != "" && upstreamOrigin != "" && response.Header.Get("Access-Control-Allow-Origin") == upstreamOrigin {
		response.Header.Set("Access-Control-Allow-Origin", browserOrigin)
	}
	stripHopHeaders(response.Header)
	contentType := response.Header.Get("Content-Type")
	if response.StatusCode >= 200 && response.StatusCode < 300 && (strings.Contains(strings.ToLower(contentType), "text/html") || strings.Contains(strings.ToLower(contentType), "text/css")) {
		rewritten, rewriteErr := s.rewriteResponse(r.Context(), route, target.URL, contentType, response.Body)
		if rewriteErr != nil {
			httpapi.Problem(w, http.StatusBadGateway, "rewrite_failed", "目标页面无法安全改写")
			return
		}
		response.Header.Del("Content-Length")
		response.Header.Del("Content-Encoding")
		response.Header.Del("ETag")
		response.Header.Del("Content-MD5")
		copyResponseHeaders(w.Header(), response.Header)
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, bytes.NewReader(rewritten))
		if r.Header.Get("Sec-Fetch-Dest") == "document" {
			s.store.UpdateCurrentURL(r.Context(), route.ContextID, target.URL.String())
			s.audit(r.Context(), route.UserID, "proxy.navigate", target.URL.String(), "allowed")
		}
		return
	}
	copyResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
	if r.Header.Get("Sec-Fetch-Dest") == "document" {
		s.store.UpdateCurrentURL(r.Context(), route.ContextID, target.URL.String())
		s.audit(r.Context(), route.UserID, "proxy.navigate", target.URL.String(), "allowed")
	}
}

func (s *Service) transport(target Target) *http.Transport {
	address := target.Addresses[0]
	dialer := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{Proxy: nil, ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.URL.Hostname()}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), strconv.Itoa(target.Port)))
	}, ResponseHeaderTimeout: 30 * time.Second, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 30 * time.Second}
}
func (s *Service) authorize(ctx context.Context, userID uuid.UUID, raw string) (Target, policy.Decision, error) {
	target, err := s.guard.Resolve(ctx, raw)
	if err != nil {
		return Target{}, policy.Decision{Reason: "invalid_target"}, err
	}
	policies, err := s.policyStore.Effective(ctx, userID)
	if err != nil {
		return Target{}, policy.Decision{}, err
	}
	return target, policy.Evaluate(policies, target.URL, target.Addresses), nil
}
func (s *Service) audit(ctx context.Context, userID uuid.UUID, eventType, target, outcome string) {
	safe := target
	if parsed, err := url.Parse(target); err == nil && parsed.IsAbs() {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		safe = parsed.String()
	}
	s.authStore.Audit(ctx, &userID, eventType, outcome, safe)
}
func (s *Service) bootstrapURL(ticket string) string {
	return s.appOrigin.Scheme + "://bootstrap." + s.proxyBaseDomain + "/__tvpn/bootstrap?ticket=" + url.QueryEscape(ticket)
}
func (s *Service) routeURL(route Route, target *url.URL) string {
	return s.appOrigin.Scheme + "://" + routeLabel(route.ID) + "." + s.proxyBaseDomain + target.EscapedPath() + querySuffix(target.RawQuery)
}
func (s *Service) mapProxyURL(ctx context.Context, value string) (string, bool) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	base := strings.ToLower(hostname(s.proxyBaseDomain))
	if !strings.HasSuffix(host, "."+base) {
		return "", false
	}
	label := strings.TrimSuffix(host, "."+base)
	route, err := s.store.RouteByLabel(ctx, label)
	if err != nil {
		return "", false
	}
	parsed.Scheme = route.Scheme
	parsed.Host = hostWithPort(route.Host, route.Port, route.Scheme)
	return parsed.String(), true
}
func (s *Service) normalizeClientURL(ctx context.Context, current Route, value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	base := strings.ToLower(hostname(s.proxyBaseDomain))
	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, "."+base) {
		return value
	}
	label := strings.TrimSuffix(host, "."+base)
	mapped, err := s.store.RouteByLabel(ctx, label)
	if err != nil || mapped.ContextID != current.ContextID || mapped.UserID != current.UserID {
		return value
	}
	websocket := parsed.Scheme == "ws" || parsed.Scheme == "wss"
	if websocket {
		if mapped.Scheme == "https" {
			parsed.Scheme = "wss"
		} else {
			parsed.Scheme = "ws"
		}
	} else {
		parsed.Scheme = mapped.Scheme
	}
	parsed.Host = hostWithPort(mapped.Host, mapped.Port, mapped.Scheme)
	return parsed.String()
}
func querySuffix(value string) string {
	if value == "" {
		return ""
	}
	return "?" + value
}
func hostWithPort(host string, port int, scheme string) string {
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
func hostname(value string) string {
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return strings.TrimSuffix(value, ".")
}
func proxyID(w http.ResponseWriter, value string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)
	if err != nil {
		httpapi.Problem(w, http.StatusBadRequest, "invalid_id", "ID 格式无效")
		return uuid.Nil, false
	}
	return id, true
}
func proxyInternal(w http.ResponseWriter) {
	httpapi.Problem(w, http.StatusInternalServerError, "internal_error", "服务器内部错误")
}
func stripHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		header.Del(name)
	}
}
func copyResponseHeaders(dst, src http.Header) {
	for name, values := range src {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "Set-Cookie" || canonical == "Content-Security-Policy" || canonical == "Content-Security-Policy-Report-Only" || canonical == "X-Frame-Options" || isHopHeader(canonical) {
			continue
		}
		for _, value := range values {
			dst.Add(canonical, value)
		}
	}
	dst.Set("Referrer-Policy", "no-referrer")
}
func isHopHeader(value string) bool {
	for _, name := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"} {
		if value == name {
			return true
		}
	}
	return false
}
