package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/FUjr/tvpn/internal/httpapi"
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
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	user, err := h.authenticate(r.Context(), input.Username, input.Password)
	if err != nil {
		h.store.Audit(r.Context(), nil, "auth.login", "failure", strings.ToLower(strings.TrimSpace(input.Username)))
		httpapi.Problem(w, r, httpapi.ErrInvalidCredentials)
		return
	}
	session, err := h.store.CreateSession(r.Context(), user, h.ttl)
	if err != nil {
		httpapi.Problem(w, r, httpapi.ErrInternal)
		return
	}
	h.setCookie(w, session.Token, session.ExpiresAt)
	h.store.Audit(r.Context(), &user.ID, "auth.login", "success", user.ID.String())
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"user": session.User, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
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
		httpapi.Problem(w, r, httpapi.ErrUnauthorized)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"user": session.User, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (h *HTTP) Logout(w http.ResponseWriter, r *http.Request) {
	if session, ok := SessionFromContext(r.Context()); ok {
		h.store.Audit(r.Context(), &session.User.ID, "auth.logout", "success", session.User.ID.String())
	}
	if cookie, err := r.Cookie(h.cookieName()); err == nil {
		_ = h.store.DeleteSession(r.Context(), cookie.Value)
	}
	h.setCookie(w, "", time.Unix(1, 0))
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := bearerToken(r.Header.Get("Authorization")); ok {
			session, err := h.store.SessionByProgramToken(r.Context(), token)
			if err != nil {
				httpapi.Problem(w, r, httpapi.ErrInvalidToken)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey, session)))
			return
		}
		cookie, err := r.Cookie(h.cookieName())
		if err != nil {
			httpapi.Problem(w, r, httpapi.ErrUnauthorized)
			return
		}
		session, err := h.store.SessionByToken(r.Context(), cookie.Value)
		if err != nil {
			httpapi.Problem(w, r, httpapi.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey, session)))
	})
}

func (h *HTTP) RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		session, ok := SessionFromContext(r.Context())
		if ok && session.Method == "token" {
			next.ServeHTTP(w, r)
			return
		}
		token := r.Header.Get("X-CSRF-Token")
		if !ok || len(token) != len(session.CSRFToken) || subtle.ConstantTimeCompare([]byte(token), []byte(session.CSRFToken)) != 1 {
			httpapi.Problem(w, r, httpapi.ErrCSRFFailed)
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

func (h *HTTP) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := SessionFromContext(r.Context())
		if !ok || !session.User.IsAdmin {
			httpapi.Problem(w, r, httpapi.ErrAdminRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *HTTP) RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, ok := SessionFromContext(r.Context())
			if !ok {
				httpapi.Problem(w, r, httpapi.ErrUnauthorized)
				return
			}
			if session.Method == "token" && !session.Scopes[scope] {
				httpapi.Problem(w, r, httpapi.ErrScopeRequired)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *HTTP) RequireBrowserSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := SessionFromContext(r.Context())
		if !ok || session.Method != "session" {
			httpapi.Problem(w, r, httpapi.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	returnValue := ""
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		returnValue = parts[1]
	}
	return returnValue, returnValue != ""
}
