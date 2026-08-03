package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserNotFound = errors.New("user not found")

type LDAPGroup struct {
	DN   string
	Name string
}
type LDAPIdentity struct {
	Username    string
	DisplayName string
	Email       string
	Groups      []LDAPGroup
}

type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email"`
	AuthSource   string    `json:"auth_source"`
	IsAdmin      bool      `json:"is_admin"`
	PasswordHash string    `json:"-"`
}

type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	User      User
	Method    string
	Scopes    map[string]bool
}

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func normalizeUsername(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func (s *Store) FindUserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := s.db.QueryRow(ctx, `SELECT id, username, display_name, email, auth_source, is_admin, COALESCE(password_hash, '')
		FROM users WHERE normalized_username=$1 AND disabled_at IS NULL`, normalizeUsername(username)).Scan(
		&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.AuthSource, &user.IsAdmin, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (s *Store) UpsertLDAPUser(ctx context.Context, identity LDAPIdentity) (User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	user := User{Username: strings.TrimSpace(identity.Username), DisplayName: strings.TrimSpace(identity.DisplayName), Email: strings.TrimSpace(identity.Email), AuthSource: "ldap"}
	err = tx.QueryRow(ctx, `INSERT INTO users (id,username,normalized_username,display_name,email,auth_source)
		VALUES ($1,$2,$3,$4,$5,'ldap') ON CONFLICT (normalized_username) DO UPDATE SET username=excluded.username,
		display_name=excluded.display_name,email=excluded.email,updated_at=now() WHERE users.auth_source='ldap'
		RETURNING id,is_admin`, uuid.New(), user.Username, normalizeUsername(user.Username), user.DisplayName, user.Email).Scan(&user.ID, &user.IsAdmin)
	if err != nil {
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM user_ldap_groups WHERE user_id=$1`, user.ID); err != nil {
		return User{}, err
	}
	for _, group := range identity.Groups {
		var groupID uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO ldap_groups (id,external_dn,name) VALUES ($1,$2,$3)
			ON CONFLICT (external_dn) DO UPDATE SET name=excluded.name,last_seen_at=now() RETURNING id`, uuid.New(), group.DN, group.Name).Scan(&groupID)
		if err != nil {
			return User{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO user_ldap_groups (user_id,group_id) VALUES ($1,$2)`, user.ID, groupID); err != nil {
			return User{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET last_login_at=now() WHERE id=$1`, user.ID); err != nil {
		return User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) CreateLocalUser(ctx context.Context, username, displayName, email, password string, admin bool) (User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	user := User{ID: uuid.New(), Username: strings.TrimSpace(username), DisplayName: strings.TrimSpace(displayName), Email: strings.TrimSpace(email), AuthSource: "local", IsAdmin: admin}
	if user.Username == "" {
		return User{}, errors.New("username is required")
	}
	_, err = s.db.Exec(ctx, `INSERT INTO users (id, username, normalized_username, display_name, email, auth_source, password_hash, is_admin)
		VALUES ($1,$2,$3,$4,$5,'local',$6,$7)`, user.ID, user.Username, normalizeUsername(user.Username), user.DisplayName, user.Email, hash, admin)
	return user, err
}

func (s *Store) EnsureBootstrapAdmin(ctx context.Context, username, password string) error {
	if strings.TrimSpace(username) == "" {
		return nil
	}
	var count int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	_, err := s.CreateLocalUser(ctx, username, username, "", password, true)
	return err
}

func (s *Store) CreateSession(ctx context.Context, user User, ttl time.Duration) (Session, error) {
	token, tokenHash, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	csrf, _, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	expires := time.Now().Add(ttl)
	_, err = s.db.Exec(ctx, `INSERT INTO sessions (token_hash,user_id,csrf_token,expires_at) VALUES ($1,$2,$3,$4)`, tokenHash, user.ID, csrf, expires)
	return Session{Token: token, CSRFToken: csrf, ExpiresAt: expires, User: user, Method: "session"}, err
}

func (s *Store) SessionByToken(ctx context.Context, token string) (Session, error) {
	hash := sha256.Sum256([]byte(token))
	var session Session
	err := s.db.QueryRow(ctx, `SELECT s.csrf_token,s.expires_at,u.id,u.username,u.display_name,u.email,u.auth_source,u.is_admin
		FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.expires_at>now() AND u.disabled_at IS NULL`, hash[:]).Scan(
		&session.CSRFToken, &session.ExpiresAt, &session.User.ID, &session.User.Username, &session.User.DisplayName,
		&session.User.Email, &session.User.AuthSource, &session.User.IsAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrInvalidCredentials
	}
	if err == nil {
		session.Method = "session"
		_, _ = s.db.Exec(ctx, `UPDATE sessions SET last_seen_at=now() WHERE token_hash=$1 AND last_seen_at<now()-interval '5 minutes'`, hash[:])
	}
	return session, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hash[:])
	return err
}

func (s *Store) Audit(ctx context.Context, userID *uuid.UUID, eventType, outcome, target string) {
	_, _ = s.db.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,outcome,target) VALUES ($1,$2,$3,$4)`, userID, eventType, outcome, target)
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
