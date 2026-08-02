package proxy

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestRewriteHTMLInjectsRuntimeAndRewritesSameOrigin(t *testing.T) {
	app, _ := url.Parse("https://app.example.com")
	service := &Service{appOrigin: app, proxyBaseDomain: "proxy.example.com"}
	target, _ := url.Parse("https://upstream.example/base/page")
	input := `<html><head><title>Test</title></head><body><a href="https://upstream.example/next?q=1">Next</a><script src="/app.js" integrity="sha256-test"></script></body></html>`
	output, err := service.rewriteHTML(context.Background(), Route{}, target, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	value := string(output)
	for _, expected := range []string{"window.__TVPN_CONFIG__", `"compatibilityMode":false`, `"proxyBaseDomain":"proxy.example.com"`, "/__tvpn/runtime.js", `href="/next?q=1"`, `src="/app.js"`} {
		if !strings.Contains(value, expected) {
			t.Fatalf("missing %q in %s", expected, value)
		}
	}
	if strings.Contains(value, "integrity=") {
		t.Fatal("stale subresource integrity was retained")
	}
}

func TestRewriteHTMLPublishesCompatibilityMode(t *testing.T) {
	app, _ := url.Parse("https://app.example.com")
	service := &Service{appOrigin: app, proxyBaseDomain: "proxy.example.com"}
	target, _ := url.Parse("https://upstream.example/")
	output, err := service.rewriteHTML(context.Background(), Route{CompatibilityMode: true}, target, []byte(`<html><head></head><body></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `"compatibilityMode":true`) {
		t.Fatalf("compatibility mode missing from runtime config: %s", output)
	}
}
