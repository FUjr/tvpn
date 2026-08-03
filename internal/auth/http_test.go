package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/FUjr/tvpn/internal/httpapi"
)

func TestProblemContentType(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)
	httpapi.Problem(recorder, request, httpapi.ErrInvalidRequest)
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("unexpected content type: %s", got)
	}
}
