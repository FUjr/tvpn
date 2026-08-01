package server

import "testing"

func TestAppHostCanBeBelowProxyBaseDomain(t *testing.T) {
	appHost := "vpn.proxy.example.com"
	proxyHost := "proxy.example.com"

	if !isProxyHost(appHost, proxyHost) {
		t.Fatal("test must cover an app host that is also below the proxy base domain")
	}
	if got := classifyRequestHost(appHost, appHost, proxyHost); got != appHostKind {
		t.Fatalf("app host was classified as %v instead of the management host", got)
	}
	if got := classifyRequestHost("route.proxy.example.com", appHost, proxyHost); got != proxyHostKind {
		t.Fatalf("proxy route host was classified as %v", got)
	}
}

func TestProxyHostMatchingRequiresLabelBoundary(t *testing.T) {
	proxyHost := "proxy.example.com"
	for _, host := range []string{"proxy.example.com", "bootstrap.proxy.example.com", "route.proxy.example.com"} {
		if !isProxyHost(host, proxyHost) {
			t.Errorf("expected %q to match proxy base domain", host)
		}
	}
	for _, host := range []string{"example.com", "notproxy.example.com", "proxy.example.com.example.net"} {
		if isProxyHost(host, proxyHost) {
			t.Errorf("did not expect %q to match proxy base domain", host)
		}
	}
}
