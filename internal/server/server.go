package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/FUjr/tvpn/internal/admin"
	"github.com/FUjr/tvpn/internal/auth"
	"github.com/FUjr/tvpn/internal/config"
	"github.com/FUjr/tvpn/internal/database"
	"github.com/FUjr/tvpn/internal/httpapi"
	"github.com/FUjr/tvpn/internal/ldapauth"
	proxyservice "github.com/FUjr/tvpn/internal/proxy"
	webassets "github.com/FUjr/tvpn/web"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg       config.Config
	db        *pgxpool.Pool
	router    http.Handler
	authHTTP  *auth.HTTP
	adminHTTP *admin.HTTP
	proxyHTTP *proxyservice.Service
}

type requestHostKind uint8

const (
	unknownHost requestHostKind = iota
	appHostKind
	proxyHostKind
)

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
	proxyHTTP, err := proxyservice.NewService(db, store, cfg.AppOrigin, cfg.ProxyBaseDomain, cfg.MasterKey, !cfg.Development, cfg.SessionTTL)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Server{cfg: cfg, db: db, authHTTP: auth.NewHTTP(store, cfg.SessionTTL, !cfg.Development, ldapService), adminHTTP: admin.NewHTTP(db, store, ldapService, proxyHTTP.UpstreamStore()), proxyHTTP: proxyHTTP}
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
			r.With(s.authHTTP.RequireBrowserSession, s.authHTTP.RequireCSRF).Post("/logout", s.authHTTP.Logout)
			r.Route("/tokens", func(r chi.Router) {
				r.Use(s.authHTTP.RequireBrowserSession)
				r.Use(s.authHTTP.RequireCSRF)
				r.Mount("/", s.authHTTP.TokenRoutes())
			})
		})
	})
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(s.authHTTP.Authenticate)
		r.Use(s.authHTTP.RequireAdmin)
		r.Use(s.authHTTP.RequireScope(auth.ScopeAdmin))
		r.Use(s.authHTTP.RequireCSRF)
		r.Mount("/", s.adminHTTP.Routes())
	})
	r.Route("/api/v1/proxy/contexts", func(r chi.Router) {
		r.Use(s.authHTTP.Authenticate)
		r.Use(s.authHTTP.RequireScope(auth.ScopeProxy))
		r.Use(s.authHTTP.RequireCSRF)
		r.Mount("/", s.proxyHTTP.AppRoutes())
	})
	r.Route("/api/v1/proxy/upstreams", func(r chi.Router) {
		r.Use(s.authHTTP.Authenticate)
		r.Use(s.authHTTP.RequireScope(auth.ScopeProxy))
		r.Get("/", s.proxyHTTP.AvailableUpstreams)
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
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httpapi.Problem(w, r, httpapi.ErrNotFound)
			return
		}
		serveWeb(w, r)
	})
	return r
}

func serveWeb(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	data, err := fs.ReadFile(webassets.Dist, "dist/"+name)
	if err != nil {
		name = "index.html"
		data, err = fs.ReadFile(webassets.Dist, "dist/index.html")
	}
	if err != nil {
		httpapi.Problem(w, r, httpapi.ErrWebUnavailable)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	_, _ = w.Write(data)
}

func (s *Server) Handler() http.Handler {
	app, _ := url.Parse(s.cfg.AppOrigin)
	configuredAppHost := strings.ToLower(app.Hostname())
	configuredProxyHost := strings.ToLower(serverHostname(s.cfg.ProxyBaseDomain))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/health/") {
			s.router.ServeHTTP(w, r)
			return
		}
		host := strings.ToLower(serverHostname(r.Host))
		switch classifyRequestHost(host, configuredAppHost, configuredProxyHost) {
		case appHostKind:
			s.router.ServeHTTP(w, r)
		case proxyHostKind:
			s.proxyHTTP.ServeHTTP(w, r)
		default:
			httpapi.Problem(w, r, httpapi.ErrMisdirectedRequest)
		}
	})
}
func (s *Server) Close() { s.db.Close() }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func serverHostname(value string) string {
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return strings.TrimSuffix(value, ".")
}

func isProxyHost(host, proxyHost string) bool {
	return host == proxyHost || strings.HasSuffix(host, "."+proxyHost)
}

func classifyRequestHost(host, configuredAppHost, configuredProxyHost string) requestHostKind {
	if host == configuredAppHost {
		return appHostKind
	}
	if isProxyHost(host, configuredProxyHost) {
		return proxyHostKind
	}
	return unknownHost
}
