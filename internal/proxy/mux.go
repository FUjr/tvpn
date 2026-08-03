package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

const (
	frameStart         byte = 1
	frameChunk         byte = 2
	frameEnd           byte = 3
	frameCancel        byte = 4
	frameResponse      byte = 11
	frameResponseChunk byte = 12
	frameResponseEnd   byte = 13
	frameResponseError byte = 14
	maxMuxRequestBody       = 32 << 20
)

type muxRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}
type muxState struct {
	meta   muxRequest
	body   bytes.Buffer
	ctx    context.Context
	cancel context.CancelFunc
}
type muxWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *muxWriter) send(ctx context.Context, kind byte, id uint32, payload []byte) error {
	frame := make([]byte, 6+len(payload))
	frame[0] = 1
	frame[1] = kind
	binary.BigEndian.PutUint32(frame[2:6], id)
	copy(frame[6:], payload)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.Write(ctx, websocket.MessageBinary, frame)
}

func (s *Service) serveMux(w http.ResponseWriter, r *http.Request, route Route) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxMuxRequestBody + (1 << 20))
	writer := &muxWriter{conn: conn}
	states := map[uint32]*muxState{}
	defer func() {
		for _, state := range states {
			state.cancel()
		}
	}()
	for {
		messageType, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if messageType != websocket.MessageBinary || len(data) < 6 || data[0] != 1 {
			_ = conn.Close(websocket.StatusUnsupportedData, "error.proxy.mux.invalid_frame")
			return
		}
		kind := data[1]
		id := binary.BigEndian.Uint32(data[2:6])
		payload := data[6:]
		switch kind {
		case frameStart:
			if _, exists := states[id]; exists {
				_ = writer.send(r.Context(), frameResponseError, id, []byte("error.proxy.mux.duplicate_stream"))
				continue
			}
			var meta muxRequest
			if json.Unmarshal(payload, &meta) != nil || meta.URL == "" || meta.Method == "" {
				_ = writer.send(r.Context(), frameResponseError, id, []byte("error.proxy.mux.invalid_metadata"))
				continue
			}
			ctx, cancel := context.WithCancel(r.Context())
			states[id] = &muxState{meta: meta, ctx: ctx, cancel: cancel}
		case frameChunk:
			state := states[id]
			if state == nil {
				continue
			}
			if state.body.Len()+len(payload) > maxMuxRequestBody {
				state.cancel()
				delete(states, id)
				_ = writer.send(r.Context(), frameResponseError, id, []byte("error.proxy.mux.body_too_large"))
				continue
			}
			_, _ = state.body.Write(payload)
		case frameEnd:
			state := states[id]
			if state == nil {
				continue
			}
			delete(states, id)
			go s.executeMux(writer, id, route, state)
		case frameCancel:
			if state := states[id]; state != nil {
				state.cancel()
				delete(states, id)
			}
		}
	}
}

func (s *Service) executeMux(writer *muxWriter, id uint32, route Route, state *muxState) {
	defer state.cancel()
	if state.meta.Method == http.MethodConnect || state.meta.Method == http.MethodTrace {
		s.muxError(writer, state.ctx, id, "error.proxy.mux.unsupported_method")
		return
	}
	state.meta.URL = s.normalizeClientURL(state.ctx, route, state.meta.URL)
	target, decision, err := s.authorize(state.ctx, route.UserID, state.meta.URL)
	if err != nil || !decision.Allowed {
		s.audit(state.ctx, route.UserID, "proxy.denied", state.meta.URL, "denied")
		s.muxError(writer, state.ctx, id, "error.proxy.target_denied")
		return
	}
	body := state.body.Bytes()
	response, finalTarget, err := s.muxRoundTrip(state.ctx, route, target, state.meta, body)
	if err != nil {
		s.muxError(writer, state.ctx, id, "error.proxy.upstream_error")
		return
	}
	defer response.Body.Close()
	headers := map[string]string{}
	for name, values := range response.Header {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "Set-Cookie" || canonical == "Tvpn-Error-Code" || isHopHeader(canonical) {
			continue
		}
		headers[name] = strings.Join(values, ", ")
	}
	visibleCookies, _ := s.store.VisibleCookies(state.ctx, s.cipher, route.ContextID, httpCookieURL(finalTarget.URL))
	cookieValues := make([]string, 0, len(visibleCookies))
	for _, cookie := range visibleCookies {
		cookieValues = append(cookieValues, cookie.Name+"="+cookie.Value)
	}
	statusText := strings.TrimPrefix(response.Status, fmt.Sprintf("%d ", response.StatusCode))
	metadata, _ := json.Marshal(map[string]any{"status": response.StatusCode, "statusText": statusText, "headers": headers, "url": finalTarget.URL.String(), "cookies": cookieValues})
	if writer.send(state.ctx, frameResponse, id, metadata) != nil {
		return
	}
	buffer := make([]byte, 32<<10)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			if writer.send(state.ctx, frameResponseChunk, id, buffer[:count]) != nil {
				return
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			s.muxError(writer, state.ctx, id, "error.proxy.mux.response_interrupted")
			return
		}
	}
	_ = writer.send(state.ctx, frameResponseEnd, id, nil)
}

func (s *Service) muxRoundTrip(ctx context.Context, route Route, target Target, meta muxRequest, body []byte) (*http.Response, Target, error) {
	method := meta.Method
	current := target
	for redirects := 0; redirects <= 10; redirects++ {
		request, err := http.NewRequestWithContext(ctx, method, current.URL.String(), bytes.NewReader(body))
		if err != nil {
			return nil, Target{}, err
		}
		for name, value := range meta.Headers {
			request.Header.Set(name, value)
		}
		stripHopHeaders(request.Header)
		request.Header.Del("Tvpn-Error-Code")
		request.Header.Del("Cookie")
		request.Header.Del("Accept-Encoding")
		cookies, err := s.store.Cookies(ctx, s.cipher, route.ContextID, httpCookieURL(current.URL))
		if err != nil {
			return nil, Target{}, err
		}
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		transport, err := s.transport(ctx, current, route)
		if err != nil {
			return nil, Target{}, err
		}
		response, err := transport.RoundTrip(request)
		transport.CloseIdleConnections()
		if err != nil {
			return nil, Target{}, err
		}
		if err := s.store.SaveCookies(ctx, s.cipher, route.ContextID, httpCookieURL(current.URL), response.Cookies()); err != nil {
			response.Body.Close()
			return nil, Target{}, err
		}
		if response.StatusCode < 300 || response.StatusCode > 399 || response.Header.Get("Location") == "" {
			return response, current, nil
		}
		location, err := current.URL.Parse(response.Header.Get("Location"))
		response.Body.Close()
		if err != nil {
			return nil, Target{}, err
		}
		next, decision, err := s.authorize(ctx, route.UserID, location.String())
		if err != nil || !decision.Allowed {
			return nil, Target{}, errors.New("redirect denied")
		}
		if response.StatusCode == http.StatusSeeOther || ((response.StatusCode == http.StatusMovedPermanently || response.StatusCode == http.StatusFound) && method == http.MethodPost) {
			method = http.MethodGet
			body = nil
		}
		current = next
	}
	return nil, Target{}, errors.New("too many redirects")
}
func (s *Service) muxError(writer *muxWriter, ctx context.Context, id uint32, message string) {
	_ = writer.send(ctx, frameResponseError, id, []byte(message))
}
func httpCookieURL(value *url.URL) *url.URL {
	copyValue := *value
	if copyValue.Scheme == "wss" {
		copyValue.Scheme = "https"
	}
	if copyValue.Scheme == "ws" {
		copyValue.Scheme = "http"
	}
	return &copyValue
}
