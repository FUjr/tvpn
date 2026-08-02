package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Target struct {
	URL       *url.URL
	Addresses []netip.Addr
	Port      int
}

type Guard struct {
	resolver    Resolver
	appHost     string
	proxyDomain string
}

func NewGuard(appOrigin, proxyBaseDomain string) (*Guard, error) {
	app, err := url.Parse(appOrigin)
	if err != nil || app.Hostname() == "" {
		return nil, errors.New("invalid app origin")
	}
	proxyHost := stripPort(proxyBaseDomain)
	if proxyHost == "" {
		return nil, errors.New("invalid proxy base domain")
	}
	return &Guard{resolver: net.DefaultResolver, appHost: strings.ToLower(app.Hostname()), proxyDomain: strings.ToLower(proxyHost)}, nil
}

func (g *Guard) Resolve(ctx context.Context, raw string) (Target, error) {
	parsed, err := url.Parse(normalizeTargetURL(raw))
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Target{}, errors.New("only absolute HTTP and HTTPS URLs are supported")
	}
	if parsed.User != nil || parsed.Hostname() == "" {
		return Target{}, errors.New("URL credentials and empty hosts are not allowed")
	}
	host, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(parsed.Hostname()), "."))
	if err != nil {
		return Target{}, errors.New("invalid internationalized host")
	}
	if host == g.appHost || host == g.proxyDomain || strings.HasSuffix(host, "."+g.proxyDomain) {
		return Target{}, errors.New("Tvpn control plane cannot be proxied")
	}
	port := 80
	if parsed.Scheme == "https" {
		port = 443
	}
	if parsed.Port() != "" {
		value, err := strconv.Atoi(parsed.Port())
		if err != nil || value < 1 || value > 65535 {
			return Target{}, errors.New("invalid port")
		}
		port = value
	}
	parsed.Host = host
	if (parsed.Scheme == "http" && port != 80) || (parsed.Scheme == "https" && port != 443) {
		parsed.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	parsed.Fragment = ""
	var addresses []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{literal.Unmap()}
	} else {
		addresses, err = g.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(addresses) == 0 {
			return Target{}, fmt.Errorf("resolve target host: %w", err)
		}
	}
	for i := range addresses {
		addresses[i] = addresses[i].Unmap()
		if hardDenied(addresses[i]) {
			return Target{}, errors.New("target address is always denied")
		}
	}
	return Target{URL: parsed, Addresses: addresses, Port: port}, nil
}

func normalizeTargetURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || strings.Contains(value, "://") {
		return value
	}
	if strings.HasPrefix(value, "//") {
		return "http:" + value
	}
	return "http://" + value
}

func hardDenied(address netip.Addr) bool {
	address = address.Unmap()
	for _, value := range []string{"169.254.169.254", "169.254.170.2", "100.100.100.200", "fd00:ec2::254"} {
		if address == netip.MustParseAddr(value) {
			return true
		}
	}
	return false
}

func stripPort(value string) string {
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return strings.TrimSuffix(value, ".")
}
