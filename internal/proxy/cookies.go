package proxy

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/publicsuffix"
)

func (s *Store) SaveCookies(ctx context.Context, cipher *Cipher, contextID uuid.UUID, origin *url.URL, cookies []*http.Cookie) error {
	for _, cookie := range cookies {
		domain := strings.ToLower(strings.TrimPrefix(cookie.Domain, "."))
		hostOnly := domain == ""
		if hostOnly {
			domain = strings.ToLower(origin.Hostname())
		} else if !domainMatches(strings.ToLower(origin.Hostname()), domain) {
			continue
		}
		if !hostOnly {
			suffix, _ := publicsuffix.PublicSuffix(domain)
			if suffix == domain {
				continue
			}
		}
		cookiePath := cookie.Path
		if cookiePath == "" {
			cookiePath = defaultCookiePath(origin.Path)
		}
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && cookie.Expires.Before(time.Now())) {
			_, err := s.db.Exec(ctx, `DELETE FROM proxy_cookies WHERE context_id=$1 AND name=$2 AND domain=$3 AND path=$4`, contextID, cookie.Name, domain, cookiePath)
			if err != nil {
				return err
			}
			continue
		}
		sealed, err := cipher.Encrypt(cookie.Value, []byte(contextID.String()+"\x00"+cookie.Name+"\x00"+domain+"\x00"+cookiePath))
		if err != nil {
			return err
		}
		var expires any
		if !cookie.Expires.IsZero() {
			expires = cookie.Expires
		}
		_, err = s.db.Exec(ctx, `INSERT INTO proxy_cookies(id,context_id,name,domain,path,value_encrypted,host_only,secure,http_only,same_site,expires_at)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)ON CONFLICT(context_id,name,domain,path)DO UPDATE SET value_encrypted=excluded.value_encrypted,host_only=excluded.host_only,secure=excluded.secure,http_only=excluded.http_only,same_site=excluded.same_site,expires_at=excluded.expires_at,updated_at=now()`, uuid.New(), contextID, cookie.Name, domain, cookiePath, sealed, hostOnly, cookie.Secure, cookie.HttpOnly, cookie.SameSite, expires)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Cookies(ctx context.Context, cipher *Cipher, contextID uuid.UUID, target *url.URL) ([]*http.Cookie, error) {
	return s.cookies(ctx, cipher, contextID, target, false)
}

func (s *Store) VisibleCookies(ctx context.Context, cipher *Cipher, contextID uuid.UUID, target *url.URL) ([]*http.Cookie, error) {
	return s.cookies(ctx, cipher, contextID, target, true)
}

func (s *Store) cookies(ctx context.Context, cipher *Cipher, contextID uuid.UUID, target *url.URL, visibleOnly bool) ([]*http.Cookie, error) {
	rows, err := s.db.Query(ctx, `SELECT name,domain,path,value_encrypted,host_only,secure,http_only FROM proxy_cookies WHERE context_id=$1 AND(expires_at IS NULL OR expires_at>now())`, contextID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []*http.Cookie
	host := strings.ToLower(target.Hostname())
	for rows.Next() {
		var name, domain, cookiePath string
		var sealed []byte
		var hostOnly, secure, httpOnly bool
		if err := rows.Scan(&name, &domain, &cookiePath, &sealed, &hostOnly, &secure, &httpOnly); err != nil {
			return nil, err
		}
		if visibleOnly && httpOnly {
			continue
		}
		if secure && target.Scheme != "https" {
			continue
		}
		if (hostOnly && host != domain) || (!hostOnly && !domainMatches(host, domain)) || !pathMatches(target.Path, cookiePath) {
			continue
		}
		plain, err := cipher.Decrypt(sealed, []byte(contextID.String()+"\x00"+name+"\x00"+domain+"\x00"+cookiePath))
		if err != nil {
			return nil, err
		}
		values = append(values, &http.Cookie{Name: name, Value: plain})
	}
	return values, rows.Err()
}
func domainMatches(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}
func pathMatches(requestPath, cookiePath string) bool {
	if requestPath == "" {
		requestPath = "/"
	}
	return requestPath == cookiePath || strings.HasPrefix(requestPath, strings.TrimSuffix(cookiePath, "/")+"/")
}
func defaultCookiePath(value string) string {
	if value == "" || value[0] != '/' {
		return "/"
	}
	directory := path.Dir(value)
	if directory == "." {
		return "/"
	}
	return directory
}
