package ldapauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/FUjr/tvpn/internal/auth"
	"github.com/go-ldap/ldap/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Settings struct {
	Enabled              bool   `json:"enabled"`
	Mode                 string `json:"mode"`
	URL                  string `json:"url"`
	StartTLS             bool   `json:"start_tls"`
	BaseDN               string `json:"base_dn"`
	BindDN               string `json:"bind_dn"`
	UserFilter           string `json:"user_filter"`
	UserDNTemplate       string `json:"user_dn_template"`
	UsernameAttribute    string `json:"username_attribute"`
	DisplayNameAttribute string `json:"display_name_attribute"`
	EmailAttribute       string `json:"email_attribute"`
	GroupMode            string `json:"group_mode"`
	GroupBaseDN          string `json:"group_base_dn"`
	GroupFilter          string `json:"group_filter"`
	GroupNameAttribute   string `json:"group_name_attribute"`
}

type Service struct {
	db               *pgxpool.Pool
	bindPasswordFile string
	caFile           string
	allowInsecure    bool
}

func New(db *pgxpool.Pool, bindPasswordFile, caFile string, allowInsecure bool) *Service {
	return &Service{db: db, bindPasswordFile: bindPasswordFile, caFile: caFile, allowInsecure: allowInsecure}
}

func (s *Service) Authenticate(ctx context.Context, username, password string) (auth.LDAPIdentity, error) {
	settings, err := s.Settings(ctx)
	if err != nil || !settings.Enabled {
		return auth.LDAPIdentity{}, auth.ErrInvalidCredentials
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return auth.LDAPIdentity{}, auth.ErrInvalidCredentials
	}

	conn, err := s.dial(settings)
	if err != nil {
		return auth.LDAPIdentity{}, fmt.Errorf("connect to LDAP: %w", err)
	}
	defer conn.Close()

	bindPassword, err := s.bindPassword()
	if err != nil {
		return auth.LDAPIdentity{}, err
	}
	var entry *ldap.Entry
	var userDN string
	if settings.Mode == "dn_template" {
		userDN = renderDN(settings.UserDNTemplate, username)
		if userDN == "" {
			return auth.LDAPIdentity{}, errors.New("LDAP user DN template is empty")
		}
		if err := conn.Bind(userDN, password); err != nil {
			return auth.LDAPIdentity{}, auth.ErrInvalidCredentials
		}
		if settings.BindDN != "" {
			if err := conn.Bind(settings.BindDN, bindPassword); err != nil {
				return auth.LDAPIdentity{}, fmt.Errorf("restore LDAP service bind: %w", err)
			}
		}
		entry, err = s.findUser(conn, settings, username)
	} else {
		if settings.BindDN == "" {
			return auth.LDAPIdentity{}, errors.New("LDAP bind DN is required for search_bind")
		}
		if err := conn.Bind(settings.BindDN, bindPassword); err != nil {
			return auth.LDAPIdentity{}, fmt.Errorf("LDAP service bind: %w", err)
		}
		entry, err = s.findUser(conn, settings, username)
		if err == nil {
			userDN = entry.DN
			err = conn.Bind(userDN, password)
		}
		if err == nil {
			err = conn.Bind(settings.BindDN, bindPassword)
		}
	}
	if err != nil {
		return auth.LDAPIdentity{}, auth.ErrInvalidCredentials
	}
	if entry == nil {
		entry = &ldap.Entry{DN: userDN}
	}
	groups, err := s.groups(conn, settings, entry)
	if err != nil {
		return auth.LDAPIdentity{}, fmt.Errorf("read LDAP groups: %w", err)
	}
	identity := auth.LDAPIdentity{
		Username:    first(entry.GetAttributeValue(settings.UsernameAttribute), username),
		DisplayName: first(entry.GetAttributeValue(settings.DisplayNameAttribute), username),
		Email:       entry.GetAttributeValue(settings.EmailAttribute), Groups: groups,
	}
	return identity, nil
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	var value Settings
	err := s.db.QueryRow(ctx, `SELECT enabled,mode,url,start_tls,base_dn,bind_dn,user_filter,user_dn_template,
		username_attribute,display_name_attribute,email_attribute,group_mode,group_base_dn,group_filter,group_name_attribute
		FROM ldap_settings WHERE singleton=true`).Scan(&value.Enabled, &value.Mode, &value.URL, &value.StartTLS, &value.BaseDN, &value.BindDN,
		&value.UserFilter, &value.UserDNTemplate, &value.UsernameAttribute, &value.DisplayNameAttribute, &value.EmailAttribute,
		&value.GroupMode, &value.GroupBaseDN, &value.GroupFilter, &value.GroupNameAttribute)
	return value, err
}

func (s *Service) dial(settings Settings) (*ldap.Conn, error) {
	parsed, err := url.Parse(settings.URL)
	if err != nil || (parsed.Scheme != "ldap" && parsed.Scheme != "ldaps") {
		return nil, errors.New("LDAP URL must use ldap or ldaps")
	}
	if parsed.Scheme == "ldap" && !settings.StartTLS && !s.allowInsecure {
		return nil, errors.New("plain LDAP is disabled")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}
	if s.caFile != "" {
		pem, err := os.ReadFile(s.caFile)
		if err != nil {
			return nil, err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("LDAP CA file contains no certificates")
		}
		tlsConfig.RootCAs = roots
	}
	conn, err := ldap.DialURL(settings.URL, ldap.DialWithTLSConfig(tlsConfig), ldap.DialWithDialer(&netDialer))
	if err != nil {
		return nil, err
	}
	conn.SetTimeout(10 * time.Second)
	if parsed.Scheme == "ldap" && settings.StartTLS {
		if err := conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

var netDialer = net.Dialer{Timeout: 10 * time.Second}

func (s *Service) findUser(conn *ldap.Conn, settings Settings, username string) (*ldap.Entry, error) {
	filter := renderFilter(settings.UserFilter, "username", username)
	request := ldap.NewSearchRequest(settings.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 10, false, filter,
		[]string{settings.UsernameAttribute, settings.DisplayNameAttribute, settings.EmailAttribute, "memberOf"}, nil)
	result, err := conn.Search(request)
	if err != nil || len(result.Entries) != 1 {
		return nil, auth.ErrInvalidCredentials
	}
	return result.Entries[0], nil
}

func (s *Service) groups(conn *ldap.Conn, settings Settings, user *ldap.Entry) ([]auth.LDAPGroup, error) {
	if settings.GroupMode == "member_of" {
		values := user.GetAttributeValues("memberOf")
		groups := make([]auth.LDAPGroup, 0, len(values))
		for _, dn := range values {
			groups = append(groups, auth.LDAPGroup{DN: dn, Name: rdnName(dn)})
		}
		return groups, nil
	}
	filter := renderFilter(settings.GroupFilter, "user_dn", user.DN)
	request := ldap.NewSearchRequest(settings.GroupBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 10, false, filter, []string{settings.GroupNameAttribute}, nil)
	result, err := conn.Search(request)
	if err != nil {
		return nil, err
	}
	groups := make([]auth.LDAPGroup, 0, len(result.Entries))
	for _, entry := range result.Entries {
		groups = append(groups, auth.LDAPGroup{DN: entry.DN, Name: first(entry.GetAttributeValue(settings.GroupNameAttribute), rdnName(entry.DN))})
	}
	return groups, nil
}

func (s *Service) bindPassword() (string, error) {
	if s.bindPasswordFile == "" {
		return "", nil
	}
	value, err := os.ReadFile(s.bindPasswordFile)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(value), "\r\n"), nil
}
func renderFilter(template, key, value string) string {
	return strings.ReplaceAll(template, "{{"+key+"}}", ldap.EscapeFilter(value))
}
func renderDN(template, username string) string {
	return strings.ReplaceAll(template, "{{username}}", ldap.EscapeDN(username))
}
func first(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
func rdnName(value string) string {
	dn, err := ldap.ParseDN(value)
	if err == nil && len(dn.RDNs) > 0 && len(dn.RDNs[0].Attributes) > 0 {
		return dn.RDNs[0].Attributes[0].Value
	}
	return value
}
