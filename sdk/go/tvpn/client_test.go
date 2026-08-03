package tvpn

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientSeparatesTvpnAndTargetAuthorizationAndClosesContext(t *testing.T) {
	var closed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/proxy/contexts/":
			if r.Header.Get("Authorization") != "Bearer tvpn_pat_test" {
				t.Fatalf("management authorization missing: %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"context":{"id":"context-id"},"route_url":"`+serverURL(r)+`/route"}`)
		case r.URL.Path == "/api/v1/proxy/contexts/context-id":
			if r.Method != http.MethodDelete {
				t.Fatalf("unexpected cleanup method: %s", r.Method)
			}
			closed.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/route":
			if r.Header.Get("Proxy-Authorization") != "Bearer tvpn_pat_test" {
				t.Fatalf("proxy authorization missing: %q", r.Header.Get("Proxy-Authorization"))
			}
			if r.Header.Get("Authorization") != "Bearer target-token" {
				t.Fatalf("target authorization changed: %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"type":"target-problem","status":404}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "tvpn_pat_test")
	response, err := client.Do(context.Background(), Request{URL: "https://api.example.com/value", Header: http.Header{"Authorization": {"Bearer target-token"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("target status was not preserved: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	if !closed.Load() {
		t.Fatal("proxy context was not closed with response body")
	}
}

func serverURL(r *http.Request) string {
	return "http://" + strings.TrimSuffix(r.Host, "/")
}

func TestPersistentSessionNavigatesWithinOneContext(t *testing.T) {
	var navigations atomic.Int32
	var closed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/proxy/contexts/":
			_, _ = io.WriteString(w, `{"context":{"id":"persistent"},"route_url":"`+serverURL(r)+`/route-one"}`)
		case "/api/v1/proxy/contexts/persistent/navigate":
			navigations.Add(1)
			_, _ = io.WriteString(w, `{"route_url":"`+serverURL(r)+`/route-two"}`)
		case "/api/v1/proxy/contexts/persistent":
			closed.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case "/route-one", "/route-two":
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "tvpn_pat_test")
	session, err := client.Open(context.Background(), "https://api.example.com/login", SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := session.Do(context.Background(), Request{URL: "https://api.example.com/login"})
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Body.Close()
	second, err := session.Do(context.Background(), Request{URL: "https://api.example.com/devices"})
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Body.Close()
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if navigations.Load() != 1 || !closed.Load() {
		t.Fatalf("unexpected lifecycle: navigations=%d closed=%v", navigations.Load(), closed.Load())
	}
}
