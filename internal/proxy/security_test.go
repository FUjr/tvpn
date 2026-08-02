package proxy

import (
	"context"
	"net/netip"
	"testing"
)

type fakeResolver struct{ addresses []netip.Addr }

func (f fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return f.addresses, nil
}
func TestGuardRejectsControlAndMetadata(t *testing.T) {
	guard, _ := NewGuard("https://app.example.com", "proxy.example.com")
	guard.resolver = fakeResolver{[]netip.Addr{netip.MustParseAddr("169.254.169.254")}}
	if _, err := guard.Resolve(context.Background(), "https://metadata.invalid"); err == nil {
		t.Fatal("metadata address accepted")
	}
	if _, err := guard.Resolve(context.Background(), "https://route.proxy.example.com"); err == nil {
		t.Fatal("proxy control domain accepted")
	}
}
func TestGuardNormalizesTarget(t *testing.T) {
	guard, _ := NewGuard("https://app.example.com", "proxy.example.com")
	guard.resolver = fakeResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8")}}
	target, err := guard.Resolve(context.Background(), "https://EXAMPLE.com:8443/a#fragment")
	if err != nil {
		t.Fatal(err)
	}
	if target.URL.String() != "https://example.com:8443/a" || target.Port != 8443 {
		t.Fatalf("unexpected target: %s %d", target.URL, target.Port)
	}
}

func TestGuardCompletesMissingHTTPScheme(t *testing.T) {
	guard, _ := NewGuard("https://app.example.com", "proxy.example.com")
	guard.resolver = fakeResolver{[]netip.Addr{netip.MustParseAddr("10.96.210.242")}}
	target, err := guard.Resolve(context.Background(), " 10.96.210.242:5666/login ")
	if err != nil {
		t.Fatal(err)
	}
	if target.URL.String() != "http://10.96.210.242:5666/login" || target.Port != 5666 {
		t.Fatalf("unexpected target: %s %d", target.URL, target.Port)
	}
}

func TestGuardPreservesExplicitHTTPS(t *testing.T) {
	guard, _ := NewGuard("https://app.example.com", "proxy.example.com")
	guard.resolver = fakeResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8")}}
	target, err := guard.Resolve(context.Background(), "https://example.com/login")
	if err != nil {
		t.Fatal(err)
	}
	if target.URL.String() != "https://example.com/login" || target.Port != 443 {
		t.Fatalf("unexpected target: %s %d", target.URL, target.Port)
	}
}
