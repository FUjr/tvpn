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
