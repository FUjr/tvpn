package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/FUjr/tvpn/internal/auth"
	"github.com/FUjr/tvpn/internal/config"
	"github.com/FUjr/tvpn/internal/database"
	"github.com/FUjr/tvpn/internal/ldapauth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg      config.Config
	db       *pgxpool.Pool
	router   http.Handler
	authHTTP *auth.HTTP
}

func New(cfg config.Config) (*Server, error) {
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	store := auth.NewStore(db)
	if cfg.BootstrapAdminUsername != "" {
		password, readErr := os.ReadFile(cfg.BootstrapAdminPasswordFile)
		if readErr != nil {
			db.Close()
			return nil, readErr
		}
		if err := store.EnsureBootstrapAdmin(context.Background(), cfg.BootstrapAdminUsername, strings.TrimRight(string(password), "\r\n")); err != nil {
			db.Close()
			return nil, err
		}
	}
	ldapService := ldapauth.New(db, cfg.LDAPBindPasswordFile, cfg.LDAPCAFile, cfg.Development && cfg.LDAPAllowInsecure)
	s := &Server{cfg: cfg, db: db, authHTTP: auth.NewHTTP(store, cfg.SessionTTL, !cfg.Development, ldapService)}
	s.router = s.routes()
	return s, nil
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/login", s.authHTTP.Login)
		r.Group(func(r chi.Router) {
			r.Use(s.authHTTP.Authenticate)
			r.Get("/session", s.authHTTP.Session)
			r.With(s.authHTTP.RequireCSRF).Post("/logout", s.authHTTP.Logout)
		})
	})
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	return r
}

func (s *Server) Handler() http.Handler { return s.router }
func (s *Server) Close()                { s.db.Close() }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
