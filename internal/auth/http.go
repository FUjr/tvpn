package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type contextKey int

const sessionContextKey contextKey = 1

type HTTP struct {
	store  *Store
	ttl    time.Duration
	secure bool
	ldap   LDAPAuthenticator
}

type LDAPAuthenticator interface {
	Authenticate(context.Context, string, string) (LDAPIdentity, error)
}

func NewHTTP(store *Store, ttl time.Duration, secure bool, ldap LDAPAuthenticator) *HTTP {
	return &HTTP{store: store, ttl: ttl, secure: secure, ldap: ldap}
}

func (h *HTTP) Login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid_request", "请求格式无效")
		return
	}
	user, err := h.authenticate(r.Context(), input.Username, input.Password)
	if err != nil {
		problem(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	session, err := h.store.CreateSession(r.Context(), user, h.ttl)
	if err != nil {
		problem(w, http.StatusInternalServerError, "internal_error", "无法创建会话")
		return
	}
	h.setCookie(w, session.Token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"user": session.User, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (h *HTTP) authenticate(ctx context.Context, username, password string) (User, error) {
	user, err := h.store.FindUserByUsername(ctx, username)
	if err == nil && user.AuthSource == "local" {
		if VerifyPassword(password, user.PasswordHash) {
			return user, nil
		}
		return User{}, ErrInvalidCredentials
	}
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return User{}, ErrInvalidCredentials
	}
	if h.ldap == nil {
		return User{}, ErrInvalidCredentials
	}
	identity, err := h.ldap.Authenticate(ctx, username, password)
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	return h.store.UpsertLDAPUser(ctx, identity)
}

func (h *HTTP) Session(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFromContext(r.Context())
	if !ok {
		problem(w, http.StatusUnauthorized, "unauthorized", "需要登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": session.User, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (h *HTTP) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(h.cookieName()); err == nil {
		_ = h.store.DeleteSession(r.Context(), cookie.Value)
	}
	h.setCookie(w, "", time.Unix(1, 0))
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(h.cookieName())
		if err != nil {
			problem(w, http.StatusUnauthorized, "unauthorized", "需要登录")
			return
		}
		session, err := h.store.SessionByToken(r.Context(), cookie.Value)
		if err != nil {
			problem(w, http.StatusUnauthorized, "unauthorized", "会话已失效")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey, session)))
	})
}

func (h *HTTP) RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := SessionFromContext(r.Context())
		token := r.Header.Get("X-CSRF-Token")
		if !ok || len(token) != len(session.CSRFToken) || subtle.ConstantTimeCompare([]byte(token), []byte(session.CSRFToken)) != 1 {
			problem(w, http.StatusForbidden, "csrf_failed", "CSRF 校验失败")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func SessionFromContext(ctx context.Context) (Session, bool) {
	value, ok := ctx.Value(sessionContextKey).(Session)
	return value, ok
}

func (h *HTTP) cookieName() string {
	if h.secure {
		return "__Host-tvpn_session"
	}
	return "tvpn_session"
}
func (h *HTTP) setCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: h.cookieName(), Value: value, Path: "/", Expires: expires, MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func problem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	writeJSON(w, status, map[string]any{"type": "https://tvpn.invalid/problems/" + code, "title": http.StatusText(status), "status": status, "detail": detail})
}
