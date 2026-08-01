package policy

import (
	"net/netip"
	"net/url"
	"testing"
)

func target(value string) *url.URL { parsed, _ := url.Parse(value); return parsed }

func TestPolicyIntersection(t *testing.T) {
	policies := []Policy{
		{Mode: Whitelist, Rules: []Rule{{Kind: DomainSuffix, Value: "example.com"}}},
		{Mode: Blacklist, Rules: []Rule{{Kind: ExactHost, Value: "blocked.example.com"}}},
	}
	public := []netip.Addr{netip.MustParseAddr("8.8.8.8")}
	if !Evaluate(policies, target("https://www.example.com/a"), public).Allowed {
		t.Fatal("allowed target rejected")
	}
	if Evaluate(policies, target("https://blocked.example.com"), public).Allowed {
		t.Fatal("blacklisted target allowed")
	}
	if Evaluate(policies, target("https://other.test"), public).Allowed {
		t.Fatal("non-whitelisted target allowed")
	}
}

func TestDenyIntranet(t *testing.T) {
	policies := []Policy{{Mode: DenyIntranet}}
	for _, value := range []string{"127.0.0.1", "10.1.2.3", "100.64.0.1", "169.254.169.254", "::1", "fd00::1"} {
		if Evaluate(policies, target("https://example.com"), []netip.Addr{netip.MustParseAddr(value)}).Allowed {
			t.Fatalf("non-public address allowed: %s", value)
		}
	}
}

func TestRuleValidation(t *testing.T) {
	if _, err := ValidateRule(DomainSuffix, "https://example.com"); err == nil {
		t.Fatal("URL accepted as domain")
	}
	if got, err := ValidateRule(CIDR, "10.0.1.1/8"); err != nil || got != "10.0.0.0/8" {
		t.Fatalf("CIDR not normalized: %s %v", got, err)
	}
}
