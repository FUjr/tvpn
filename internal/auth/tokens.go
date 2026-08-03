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
)

const (
	ScopeProxy = "proxy"
	ScopeAdmin = "admin"
)

var ErrTokenNotFound = errors.New("program token not found")

type ProgramToken struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func ValidateScopes(scopes []string, admin bool) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope != ScopeProxy && scope != ScopeAdmin {
			return nil, errors.New("unsupported scope")
		}
		if scope == ScopeAdmin && !admin {
			return nil, errors.New("admin scope requires administrator")
		}
		if !seen[scope] {
			seen[scope] = true
			result = append(result, scope)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("at least one scope is required")
	}
	return result, nil
}

func (s *Store) CreateProgramToken(ctx context.Context, user User, name string, scopes []string, expiresAt time.Time) (ProgramToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 || !expiresAt.After(time.Now()) || expiresAt.After(time.Now().Add(366*24*time.Hour)) {
		return ProgramToken{}, "", errors.New("invalid token metadata")
	}
	scopes, err := ValidateScopes(scopes, user.IsAdmin)
	if err != nil {
		return ProgramToken{}, "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return ProgramToken{}, "", err
	}
	plain := "tvpn_pat_" + base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(plain))
	value := ProgramToken{ID: uuid.New(), Name: name, Prefix: plain[:17], Scopes: scopes, ExpiresAt: expiresAt, CreatedAt: time.Now()}
	_, err = s.db.Exec(ctx, `INSERT INTO program_tokens(id,token_hash,token_prefix,user_id,name,scopes,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, value.ID, hash[:], value.Prefix, user.ID, value.Name, value.Scopes, value.ExpiresAt, value.CreatedAt)
	return value, plain, err
}

func (s *Store) ProgramTokens(ctx context.Context, userID uuid.UUID) ([]ProgramToken, error) {
	rows, err := s.db.Query(ctx, `SELECT id,name,token_prefix,scopes,expires_at,last_used_at,created_at FROM program_tokens WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []ProgramToken{}
	for rows.Next() {
		var value ProgramToken
		if err := rows.Scan(&value.ID, &value.Name, &value.Prefix, &value.Scopes, &value.ExpiresAt, &value.LastUsedAt, &value.CreatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) RevokeProgramToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `UPDATE program_tokens SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, tokenID, userID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrTokenNotFound
	}
	return err
}

func (s *Store) SessionByProgramToken(ctx context.Context, token string) (Session, error) {
	if !strings.HasPrefix(token, "tvpn_pat_") {
		return Session{}, ErrInvalidCredentials
	}
	hash := sha256.Sum256([]byte(token))
	var session Session
	var scopes []string
	err := s.db.QueryRow(ctx, `SELECT t.expires_at,t.scopes,u.id,u.username,u.display_name,u.email,u.auth_source,u.is_admin
		FROM program_tokens t JOIN users u ON u.id=t.user_id
		WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND t.expires_at>now() AND u.disabled_at IS NULL`, hash[:]).Scan(
		&session.ExpiresAt, &scopes, &session.User.ID, &session.User.Username, &session.User.DisplayName,
		&session.User.Email, &session.User.AuthSource, &session.User.IsAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}
	session.Method = "token"
	session.Scopes = map[string]bool{}
	for _, scope := range scopes {
		session.Scopes[scope] = true
	}
	_, _ = s.db.Exec(ctx, `UPDATE program_tokens SET last_used_at=now() WHERE token_hash=$1 AND(last_used_at IS NULL OR last_used_at<now()-interval '5 minutes')`, hash[:])
	return session, nil
}
