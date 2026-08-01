package policy

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Mode string

const (
	DenyIntranet Mode = "deny_intranet"
	Whitelist    Mode = "whitelist"
	Blacklist    Mode = "blacklist"
	DenyAll      Mode = "deny_all"
)

type RuleKind string

const (
	ExactHost    RuleKind = "exact_host"
	DomainSuffix RuleKind = "domain_suffix"
	CIDR         RuleKind = "cidr"
	URLPrefix    RuleKind = "url_prefix"
)

type Rule struct {
	ID    uuid.UUID `json:"id"`
	Kind  RuleKind  `json:"kind"`
	Value string    `json:"value"`
}

type Policy struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Mode        Mode      `json:"mode"`
	Enabled     bool      `json:"enabled"`
	Rules       []Rule    `json:"rules"`
}

type Decision struct {
	Allowed  bool
	Reason   string
	PolicyID uuid.UUID
}

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Effective(ctx context.Context, userID uuid.UUID) ([]Policy, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT p.id,p.name,p.description,p.mode,p.enabled
		FROM policies p WHERE p.enabled=true AND (
		EXISTS (SELECT 1 FROM user_policies up WHERE up.policy_id=p.id AND up.user_id=$1)
		OR EXISTS (SELECT 1 FROM user_ldap_groups ug JOIN ldap_group_policies gp ON gp.group_id=ug.group_id
		WHERE ug.user_id=$1 AND gp.policy_id=p.id)) ORDER BY p.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []Policy
	for rows.Next() {
		item := Policy{Rules: []Rule{}}
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Mode, &item.Enabled); err != nil {
			return nil, err
		}
		policies = append(policies, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range policies {
		rules, err := s.db.Query(ctx, `SELECT id,kind,value FROM policy_rules WHERE policy_id=$1 ORDER BY kind,value`, policies[i].ID)
		if err != nil {
			return nil, err
		}
		for rules.Next() {
			var rule Rule
			if err := rules.Scan(&rule.ID, &rule.Kind, &rule.Value); err != nil {
				rules.Close()
				return nil, err
			}
			policies[i].Rules = append(policies[i].Rules, rule)
		}
		err = rules.Err()
		rules.Close()
		if err != nil {
			return nil, err
		}
	}
	return policies, nil
}

func Evaluate(policies []Policy, target *url.URL, addresses []netip.Addr) Decision {
	if len(policies) == 0 {
		return Decision{Reason: "no_policy"}
	}
	for _, item := range policies {
		matched := matchesAny(item.Rules, target, addresses)
		switch item.Mode {
		case DenyAll:
			return Decision{Reason: "deny_all", PolicyID: item.ID}
		case Whitelist:
			if !matched {
				return Decision{Reason: "not_whitelisted", PolicyID: item.ID}
			}
		case Blacklist:
			if matched {
				return Decision{Reason: "blacklisted", PolicyID: item.ID}
			}
		case DenyIntranet:
			for _, address := range addresses {
				if IsNonPublic(address) {
					return Decision{Reason: "intranet_address", PolicyID: item.ID}
				}
			}
			if matched {
				return Decision{Reason: "custom_intranet", PolicyID: item.ID}
			}
		default:
			return Decision{Reason: "invalid_policy", PolicyID: item.ID}
		}
	}
	return Decision{Allowed: true, Reason: "allowed"}
}

func ValidateRule(kind RuleKind, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("rule value is required")
	}
	switch kind {
	case ExactHost, DomainSuffix:
		value = strings.ToLower(strings.TrimSuffix(value, "."))
		if strings.ContainsAny(value, "/:@") {
			return "", errors.New("host rule must not include scheme, path, port, or credentials")
		}
	case CIDR:
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return "", errors.New("invalid CIDR")
		}
		value = prefix.Masked().String()
	case URLPrefix:
		parsed, err := url.Parse(value)
		if err != nil || !parsed.IsAbs() || !allowedScheme(parsed.Scheme) || parsed.User != nil {
			return "", errors.New("invalid URL prefix")
		}
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Fragment = ""
		parsed.RawQuery = ""
		value = parsed.String()
	default:
		return "", fmt.Errorf("unsupported rule kind %q", kind)
	}
	return value, nil
}

func allowedScheme(value string) bool {
	return value == "http" || value == "https" || value == "ws" || value == "wss"
}
func matchesAny(rules []Rule, target *url.URL, addresses []netip.Addr) bool {
	for _, rule := range rules {
		if match(rule, target, addresses) {
			return true
		}
	}
	return false
}
func match(rule Rule, target *url.URL, addresses []netip.Addr) bool {
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	switch rule.Kind {
	case ExactHost:
		return host == rule.Value
	case DomainSuffix:
		return host == rule.Value || strings.HasSuffix(host, "."+rule.Value)
	case URLPrefix:
		prefix, err := url.Parse(rule.Value)
		return err == nil && target.Scheme == prefix.Scheme && strings.EqualFold(target.Host, prefix.Host) && strings.HasPrefix(target.EscapedPath(), prefix.EscapedPath())
	case CIDR:
		prefix, err := netip.ParsePrefix(rule.Value)
		if err != nil {
			return false
		}
		for _, address := range addresses {
			if prefix.Contains(address.Unmap()) {
				return true
			}
		}
	}
	return false
}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"), netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"), netip.MustParsePrefix("2001:db8::/32"),
}

func IsNonPublic(address netip.Addr) bool {
	address = address.Unmap()
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return !address.IsGlobalUnicast()
}
