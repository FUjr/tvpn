package proxy

import (
	"net/http"
	"testing"
)

func TestTranslateCORSPreservesLegacyBehaviorWhenCompatibilityModeIsOff(t *testing.T) {
	response := http.Header{"Access-Control-Allow-Origin": []string{"https://assets.example.com"}}
	translateCORS(response, http.Header{}, "https://route.proxy.example.com", "https://assets.example.com", false)
	if got := response.Get("Access-Control-Allow-Origin"); got != "https://route.proxy.example.com" {
		t.Fatalf("unexpected translated origin %q", got)
	}

	response = http.Header{}
	translateCORS(response, http.Header{}, "https://route.proxy.example.com", "https://assets.example.com", false)
	if got := response.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("legacy mode added an origin: %q", got)
	}
}

func TestTranslateCORSCompatibilityModeAllowsOnlyRequesterOrigin(t *testing.T) {
	response := http.Header{"Vary": []string{"Accept-Encoding"}}
	request := http.Header{
		"Access-Control-Request-Method":  []string{"POST"},
		"Access-Control-Request-Headers": []string{"x-client-id, content-type"},
	}
	translateCORS(response, request, "https://route.proxy.example.com", "https://www.example.com", true)
	translateCORS(response, request, "https://route.proxy.example.com", "https://www.example.com", true)

	checks := map[string]string{
		"Access-Control-Allow-Origin":      "https://route.proxy.example.com",
		"Access-Control-Allow-Credentials": "true",
		"Access-Control-Allow-Methods":     "POST",
		"Access-Control-Allow-Headers":     "x-client-id, content-type",
	}
	for name, want := range checks {
		if got := response.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	if got := response.Values("Vary"); len(got) != 2 || got[0] != "Accept-Encoding" || got[1] != "Origin" {
		t.Fatalf("unexpected Vary values: %#v", got)
	}
}

func TestTranslateCORSRequiresMappedOrigins(t *testing.T) {
	response := http.Header{}
	translateCORS(response, http.Header{}, "https://route.proxy.example.com", "", true)
	if response.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("compatibility mode allowed an unmapped origin")
	}
}

func TestCompatibilityRequestRequiresModeAndOrigin(t *testing.T) {
	service := &Service{}
	request, _ := http.NewRequest(http.MethodGet, "https://target.proxy.example.com/app.js", nil)
	if service.compatibilityRequestAllowed(request, Route{CompatibilityMode: true}) {
		t.Fatal("compatibility request without an origin was allowed")
	}
	request.Header.Set("Origin", "https://source.proxy.example.com")
	if service.compatibilityRequestAllowed(request, Route{}) {
		t.Fatal("compatibility request was allowed while the mode was disabled")
	}
	request.URL.Path = "/__tvpn/resolve"
	if service.compatibilityRequestAllowed(request, Route{CompatibilityMode: true}) {
		t.Fatal("compatibility request was allowed to access a control endpoint")
	}
	request.URL.Path = "/app.js"
	request.Header.Set("Sec-Fetch-Dest", "document")
	if service.compatibilityRequestAllowed(request, Route{CompatibilityMode: true}) {
		t.Fatal("compatibility request was allowed for a document navigation")
	}
}

func TestProxyBearerToken(t *testing.T) {
	if token, ok := proxyBearerToken("Bearer tvpn_pat_value"); !ok || token != "tvpn_pat_value" {
		t.Fatalf("unexpected token: %q, %v", token, ok)
	}
	if _, ok := proxyBearerToken("Basic value"); ok {
		t.Fatal("non-Bearer proxy authentication was accepted")
	}
}
