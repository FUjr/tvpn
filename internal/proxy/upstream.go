package proxy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UpstreamType string

const (
	UpstreamHTTP   UpstreamType = "http"
	UpstreamSOCKS5 UpstreamType = "socks5"
)

type Upstream struct {
	ID                 uuid.UUID    `json:"id"`
	Name               string       `json:"name"`
	Type               UpstreamType `json:"type"`
	Host               string       `json:"host"`
	Port               int          `json:"port"`
	Username           string       `json:"username"`
	PasswordConfigured bool         `json:"password_configured"`
	Enabled            bool         `json:"enabled"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
	passwordEncrypted  []byte
}

type UpstreamInput struct {
	Name          string       `json:"name"`
	Type          UpstreamType `json:"type"`
	Host          string       `json:"host"`
	Port          int          `json:"port"`
	Username      string       `json:"username"`
	Password      string       `json:"password"`
	ClearPassword bool         `json:"clear_password"`
	Enabled       bool         `json:"enabled"`
}

type UpstreamStore struct {
	db     *pgxpool.Pool
	cipher *Cipher
}

func NewUpstreamStore(db *pgxpool.Pool, cipher *Cipher) *UpstreamStore {
	return &UpstreamStore{db: db, cipher: cipher}
}

func normalizeUpstreamInput(input *UpstreamInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Host = strings.TrimSpace(strings.TrimSuffix(input.Host, "."))
	if strings.HasPrefix(input.Host, "[") && strings.HasSuffix(input.Host, "]") {
		input.Host = strings.TrimSuffix(strings.TrimPrefix(input.Host, "["), "]")
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Name == "" {
		return errors.New("name is required")
	}
	if input.Type != UpstreamHTTP && input.Type != UpstreamSOCKS5 {
		return errors.New("proxy type must be http or socks5")
	}
	if input.Host == "" || strings.ContainsAny(input.Host, "/?#@ \\") {
		return errors.New("proxy host is invalid")
	}
	if strings.Contains(input.Host, ":") && net.ParseIP(input.Host) == nil {
		return errors.New("proxy host must not include a port")
	}
	if input.Port < 1 || input.Port > 65535 {
		return errors.New("proxy port is invalid")
	}
	if input.Password != "" && input.Username == "" {
		return errors.New("username is required when password is configured")
	}
	return nil
}

func (s *UpstreamStore) List(ctx context.Context) ([]Upstream, error) {
	rows, err := s.db.Query(ctx, `SELECT id,name,proxy_type,host,port,username,password_encrypted IS NOT NULL,enabled,created_at,updated_at FROM upstream_proxies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []Upstream{}
	for rows.Next() {
		var value Upstream
		if err := rows.Scan(&value.ID, &value.Name, &value.Type, &value.Host, &value.Port, &value.Username, &value.PasswordConfigured, &value.Enabled, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *UpstreamStore) Create(ctx context.Context, input UpstreamInput) (uuid.UUID, error) {
	if err := normalizeUpstreamInput(&input); err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	var password []byte
	var err error
	if input.Password != "" {
		password, err = s.cipher.Encrypt(input.Password, upstreamAAD(id))
		if err != nil {
			return uuid.Nil, err
		}
	}
	_, err = s.db.Exec(ctx, `INSERT INTO upstream_proxies(id,name,proxy_type,host,port,username,password_encrypted,enabled)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, input.Name, input.Type, input.Host, input.Port, input.Username, password, input.Enabled)
	return id, err
}

func (s *UpstreamStore) Update(ctx context.Context, id uuid.UUID, input UpstreamInput) error {
	if err := normalizeUpstreamInput(&input); err != nil {
		return err
	}
	var password []byte
	var err error
	if input.Password != "" {
		password, err = s.cipher.Encrypt(input.Password, upstreamAAD(id))
		if err != nil {
			return err
		}
	}
	tag, err := s.db.Exec(ctx, `UPDATE upstream_proxies SET name=$2,proxy_type=$3,host=$4,port=$5,username=$6,
		password_encrypted=CASE WHEN $7::bytea IS NOT NULL THEN $7 WHEN $8 THEN NULL ELSE password_encrypted END,
		enabled=$9,updated_at=now() WHERE id=$1`, id, input.Name, input.Type, input.Host, input.Port, input.Username, password, input.ClearPassword, input.Enabled)
	if err == nil && tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}

func (s *UpstreamStore) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM proxy_contexts WHERE upstream_proxy_id=$1`, id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM upstream_proxies WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func (s *UpstreamStore) Effective(ctx context.Context, userID uuid.UUID) ([]Upstream, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT p.id,p.name,p.proxy_type,p.host,p.port,p.username,p.password_encrypted IS NOT NULL,p.enabled,p.created_at,p.updated_at
		FROM upstream_proxies p WHERE p.enabled AND (
		EXISTS(SELECT 1 FROM user_upstream_proxies up WHERE up.user_id=$1 AND up.upstream_proxy_id=p.id) OR
		EXISTS(SELECT 1 FROM user_ldap_groups ug JOIN ldap_group_upstream_proxies gp ON gp.group_id=ug.group_id WHERE ug.user_id=$1 AND gp.upstream_proxy_id=p.id)) ORDER BY p.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []Upstream{}
	for rows.Next() {
		var value Upstream
		if err := rows.Scan(&value.ID, &value.Name, &value.Type, &value.Host, &value.Port, &value.Username, &value.PasswordConfigured, &value.Enabled, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *UpstreamStore) Authorized(ctx context.Context, userID, id uuid.UUID) (*Upstream, error) {
	var value Upstream
	err := s.db.QueryRow(ctx, `SELECT p.id,p.name,p.proxy_type,p.host,p.port,p.username,p.password_encrypted,p.enabled,p.created_at,p.updated_at
		FROM upstream_proxies p WHERE p.id=$2 AND p.enabled AND (
		EXISTS(SELECT 1 FROM user_upstream_proxies up WHERE up.user_id=$1 AND up.upstream_proxy_id=p.id) OR
		EXISTS(SELECT 1 FROM user_ldap_groups ug JOIN ldap_group_upstream_proxies gp ON gp.group_id=ug.group_id WHERE ug.user_id=$1 AND gp.upstream_proxy_id=p.id))`, userID, id).
		Scan(&value.ID, &value.Name, &value.Type, &value.Host, &value.Port, &value.Username, &value.passwordEncrypted, &value.Enabled, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return nil, err
	}
	value.PasswordConfigured = len(value.passwordEncrypted) > 0
	return &value, nil
}

func (s *UpstreamStore) Password(value *Upstream) (string, error) {
	if len(value.passwordEncrypted) == 0 {
		return "", nil
	}
	return s.cipher.Decrypt(value.passwordEncrypted, upstreamAAD(value.ID))
}

func (s *UpstreamStore) ProxyURL(value *Upstream) (*url.URL, error) {
	password, err := s.Password(value)
	if err != nil {
		return nil, err
	}
	result := &url.URL{Scheme: string(value.Type), Host: net.JoinHostPort(value.Host, strconv.Itoa(value.Port))}
	if value.Username != "" {
		result.User = url.UserPassword(value.Username, password)
	}
	return result, nil
}

func upstreamAAD(id uuid.UUID) []byte { return []byte("upstream-proxy:" + id.String()) }
