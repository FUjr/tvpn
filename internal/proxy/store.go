package proxy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Context struct {
	ID              uuid.UUID  `json:"id"`
	CurrentURL      string     `json:"current_url"`
	UpstreamProxyID *uuid.UUID `json:"upstream_proxy_id"`
	CreatedAt       time.Time  `json:"created_at"`
	LastActiveAt    time.Time  `json:"last_active_at"`
}
type Route struct {
	ID              uuid.UUID
	ContextID       uuid.UUID
	UserID          uuid.UUID
	Scheme          string
	Host            string
	Port            int
	CurrentURL      string
	UpstreamProxyID *uuid.UUID
}
type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) CreateContext(ctx context.Context, userID uuid.UUID, target *url.URL, upstreamProxyID *uuid.UUID) (Context, Route, string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Context{}, Route{}, "", err
	}
	defer tx.Rollback(ctx)
	contextValue := Context{ID: uuid.New(), CurrentURL: target.String(), UpstreamProxyID: upstreamProxyID, CreatedAt: time.Now(), LastActiveAt: time.Now()}
	if _, err = tx.Exec(ctx, `INSERT INTO proxy_contexts(id,user_id,current_url,upstream_proxy_id)VALUES($1,$2,$3,$4)`, contextValue.ID, userID, target.String(), upstreamProxyID); err != nil {
		return Context{}, Route{}, "", err
	}
	route, err := createRoute(ctx, tx, contextValue.ID, userID, target)
	if err != nil {
		return Context{}, Route{}, "", err
	}
	ticket, hash, err := randomToken()
	if err != nil {
		return Context{}, Route{}, "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO proxy_tickets(token_hash,user_id,context_id,expires_at)VALUES($1,$2,$3,now()+interval '2 minutes')`, hash, userID, contextValue.ID); err != nil {
		return Context{}, Route{}, "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return Context{}, Route{}, "", err
	}
	return contextValue, route, ticket, nil
}

func (s *Store) CreateTicket(ctx context.Context, userID, contextID uuid.UUID) (string, error) {
	var owner uuid.UUID
	if err := s.db.QueryRow(ctx, `SELECT user_id FROM proxy_contexts WHERE id=$1 AND closed_at IS NULL`, contextID).Scan(&owner); err != nil || owner != userID {
		return "", errors.New("proxy context not found")
	}
	token, hash, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO proxy_tickets(token_hash,user_id,context_id,expires_at) VALUES($1,$2,$3,now()+interval '2 minutes')`, hash, userID, contextID)
	return token, err
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func createRoute(ctx context.Context, q querier, contextID, userID uuid.UUID, target *url.URL) (Route, error) {
	port := 80
	if target.Scheme == "https" {
		port = 443
	}
	if target.Port() != "" {
		port, _ = strconv.Atoi(target.Port())
	}
	route := Route{ID: uuid.New(), ContextID: contextID, UserID: userID, Scheme: target.Scheme, Host: target.Hostname(), Port: port}
	err := q.QueryRow(ctx, `INSERT INTO proxy_routes(id,context_id,upstream_scheme,upstream_host,upstream_port)VALUES($1,$2,$3,$4,$5)
	ON CONFLICT(context_id,upstream_scheme,upstream_host,upstream_port)DO UPDATE SET upstream_host=excluded.upstream_host RETURNING id`, route.ID, contextID, route.Scheme, route.Host, route.Port).Scan(&route.ID)
	return route, err
}

func (s *Store) ResolveRoute(ctx context.Context, contextID, userID uuid.UUID, target *url.URL) (Route, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Route{}, err
	}
	defer tx.Rollback(ctx)
	var owner uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT user_id FROM proxy_contexts WHERE id=$1 AND closed_at IS NULL`, contextID).Scan(&owner); err != nil || owner != userID {
		return Route{}, errors.New("proxy context not found")
	}
	route, err := createRoute(ctx, tx, contextID, userID, target)
	if err != nil {
		return Route{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Route{}, err
	}
	return route, nil
}

func (s *Store) RouteByLabel(ctx context.Context, label string) (Route, error) {
	id, err := parseLabel(label)
	if err != nil {
		return Route{}, err
	}
	var route Route
	err = s.db.QueryRow(ctx, `SELECT r.id,r.context_id,c.user_id,r.upstream_scheme,r.upstream_host,r.upstream_port,c.current_url,c.upstream_proxy_id FROM proxy_routes r JOIN proxy_contexts c ON c.id=r.context_id WHERE r.id=$1 AND c.closed_at IS NULL`, id).Scan(&route.ID, &route.ContextID, &route.UserID, &route.Scheme, &route.Host, &route.Port, &route.CurrentURL, &route.UpstreamProxyID)
	return route, err
}

func (s *Store) ConsumeTicket(ctx context.Context, token string, ttl time.Duration) (string, uuid.UUID, uuid.UUID, error) {
	hash := sha256.Sum256([]byte(token))
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", uuid.Nil, uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	var userID, contextID uuid.UUID
	err = tx.QueryRow(ctx, `UPDATE proxy_tickets SET used_at=now() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now() RETURNING user_id,context_id`, hash[:]).Scan(&userID, &contextID)
	if err != nil {
		return "", uuid.Nil, uuid.Nil, err
	}
	session, sessionHash, err := randomToken()
	if err != nil {
		return "", uuid.Nil, uuid.Nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO proxy_sessions(token_hash,user_id,expires_at)VALUES($1,$2,$3)`, sessionHash, userID, time.Now().Add(ttl)); err != nil {
		return "", uuid.Nil, uuid.Nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", uuid.Nil, uuid.Nil, err
	}
	return session, userID, contextID, nil
}

func (s *Store) AuthenticateProxy(ctx context.Context, token string) (uuid.UUID, error) {
	hash := sha256.Sum256([]byte(token))
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT user_id FROM proxy_sessions WHERE token_hash=$1 AND expires_at>now()`, hash[:]).Scan(&id)
	return id, err
}
func (s *Store) Context(ctx context.Context, id, userID uuid.UUID) (Context, error) {
	var value Context
	err := s.db.QueryRow(ctx, `SELECT id,current_url,upstream_proxy_id,created_at,last_active_at FROM proxy_contexts WHERE id=$1 AND user_id=$2 AND closed_at IS NULL`, id, userID).Scan(&value.ID, &value.CurrentURL, &value.UpstreamProxyID, &value.CreatedAt, &value.LastActiveAt)
	return value, err
}
func (s *Store) CloseContext(ctx context.Context, id, userID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `UPDATE proxy_contexts SET closed_at=now()WHERE id=$1 AND user_id=$2 AND closed_at IS NULL`, id, userID)
	if err == nil && tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}
func (s *Store) UpdateCurrentURL(ctx context.Context, id uuid.UUID, value string) {
	_, _ = s.db.Exec(ctx, `UPDATE proxy_contexts SET current_url=$2,last_active_at=now()WHERE id=$1`, id, value)
}
func randomToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}
func routeLabel(id uuid.UUID) string { return strings.ReplaceAll(id.String(), "-", "") }
func parseLabel(value string) (uuid.UUID, error) {
	if len(value) != 32 {
		return uuid.Nil, errors.New("invalid route label")
	}
	return uuid.Parse(value[0:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:])
}
