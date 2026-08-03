package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/FUjr/tvpn/internal/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *HTTP) TokenRoutes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", h.listTokens)
	r.Post("/", h.createToken)
	r.Delete("/{id}", h.revokeToken)
	return r
}

func (h *HTTP) listTokens(w http.ResponseWriter, r *http.Request) {
	session, _ := SessionFromContext(r.Context())
	values, err := h.store.ProgramTokens(r.Context(), session.User.ID)
	if err != nil {
		httpapi.Problem(w, r, httpapi.ErrInternal)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (h *HTTP) createToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	session, _ := SessionFromContext(r.Context())
	expiresAt := time.Now().Add(90 * 24 * time.Hour)
	if input.ExpiresAt != nil {
		expiresAt = *input.ExpiresAt
	}
	if strings.TrimSpace(input.Name) == "" {
		httpapi.Problem(w, r, httpapi.ErrInvalidRequest)
		return
	}
	if _, err := ValidateScopes(input.Scopes, session.User.IsAdmin); err != nil {
		httpapi.Problem(w, r, httpapi.ErrInvalidScope)
		return
	}
	value, plain, err := h.store.CreateProgramToken(r.Context(), session.User, input.Name, input.Scopes, expiresAt)
	if err != nil {
		httpapi.Problem(w, r, httpapi.ErrInvalidRequest)
		return
	}
	h.store.Audit(r.Context(), &session.User.ID, "auth.token.create", "success", value.ID.String())
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"token": value, "secret": plain})
}

func (h *HTTP) revokeToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpapi.Problem(w, r, httpapi.ErrInvalidID)
		return
	}
	session, _ := SessionFromContext(r.Context())
	if err := h.store.RevokeProgramToken(r.Context(), session.User.ID, id); err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			httpapi.Problem(w, r, httpapi.ErrTokenNotFound)
		} else {
			httpapi.Problem(w, r, httpapi.ErrInternal)
		}
		return
	}
	h.store.Audit(r.Context(), &session.User.ID, "auth.token.revoke", "success", id.String())
	w.WriteHeader(http.StatusNoContent)
}
