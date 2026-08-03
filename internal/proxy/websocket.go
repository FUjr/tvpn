package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/FUjr/tvpn/internal/httpapi"
	"github.com/FUjr/tvpn/internal/policy"
	"github.com/coder/websocket"
	"github.com/google/uuid"
)

func (s *Service) serveWebSocket(w http.ResponseWriter, r *http.Request, route Route) {
	raw := s.normalizeClientURL(r.Context(), route, r.URL.Query().Get("target"))
	target, decision, err := s.authorizeWebSocket(r.Context(), route.UserID, raw)
	if err != nil || !decision.Allowed {
		s.audit(r.Context(), route.UserID, "proxy.websocket.denied", raw, "denied")
		httpapi.Problem(w, r, httpapi.ErrTargetDenied)
		return
	}
	headers := http.Header{}
	headers.Set("Origin", route.Scheme+"://"+hostWithPort(route.Host, route.Port, route.Scheme))
	cookies, err := s.store.Cookies(r.Context(), s.cipher, route.ContextID, httpCookieURL(target.URL))
	if err != nil {
		proxyInternal(w, r)
		return
	}
	for _, cookie := range cookies {
		headers.Add("Cookie", cookie.String())
	}
	protocols := splitProtocols(r.Header.Get("Sec-WebSocket-Protocol"))
	transport, err := s.transport(r.Context(), target, route)
	if err != nil {
		httpapi.Problem(w, r, httpapi.ErrUpstreamProxyUnavailable)
		return
	}
	upstream, response, err := websocket.Dial(r.Context(), target.URL.String(), &websocket.DialOptions{HTTPClient: &http.Client{Transport: transport}, HTTPHeader: headers, Subprotocols: protocols, CompressionMode: websocket.CompressionDisabled})
	transport.CloseIdleConnections()
	if err != nil {
		httpapi.Problem(w, r, httpapi.ErrWebSocketUpstreamFailed)
		return
	}
	defer upstream.CloseNow()
	if response != nil {
		_ = s.store.SaveCookies(r.Context(), s.cipher, route.ContextID, httpCookieURL(target.URL), response.Cookies())
	}
	options := &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled}
	if upstream.Subprotocol() != "" {
		options.Subprotocols = []string{upstream.Subprotocol()}
	}
	downstream, err := websocket.Accept(w, r, options)
	if err != nil {
		return
	}
	defer downstream.CloseNow()
	upstream.SetReadLimit(32 << 20)
	downstream.SetReadLimit(32 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	results := make(chan error, 2)
	go websocketPump(ctx, downstream, upstream, results)
	go websocketPump(ctx, upstream, downstream, results)
	first := <-results
	cancel()
	status := websocket.CloseStatus(first)
	if status < 0 {
		status = websocket.StatusNormalClosure
	}
	reason := ""
	var closeErr websocket.CloseError
	if errors.As(first, &closeErr) {
		reason = closeErr.Reason
	}
	_ = downstream.Close(status, reason)
	_ = upstream.Close(status, reason)
}

func websocketPump(ctx context.Context, destination, source *websocket.Conn, results chan<- error) {
	for {
		messageType, data, err := source.Read(ctx)
		if err != nil {
			results <- err
			return
		}
		if err := destination.Write(ctx, messageType, data); err != nil {
			results <- err
			return
		}
	}
}
func splitProtocols(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
func (s *Service) authorizeWebSocket(ctx context.Context, userID uuid.UUID, raw string) (Target, policy.Decision, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return Target{}, policy.Decision{Reason: "invalid_target"}, errors.New("only ws and wss URLs are supported")
	}
	httpURL := *parsed
	if parsed.Scheme == "ws" {
		httpURL.Scheme = "http"
	} else {
		httpURL.Scheme = "https"
	}
	target, err := s.guard.Resolve(ctx, httpURL.String())
	if err != nil {
		return Target{}, policy.Decision{Reason: "invalid_target"}, err
	}
	target.URL.Scheme = parsed.Scheme
	policies, err := s.policyStore.Effective(ctx, userID)
	if err != nil {
		return Target{}, policy.Decision{}, err
	}
	return target, policy.Evaluate(policies, target.URL, target.Addresses), nil
}
