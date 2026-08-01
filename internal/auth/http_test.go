package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FUjr/tvpn/internal/httpapi"
)

func TestProblemContentType(t *testing.T) {
	recorder := httptest.NewRecorder()
	httpapi.Problem(recorder, http.StatusBadRequest, "invalid_request", "bad request")
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("unexpected content type: %s", got)
	}
}
