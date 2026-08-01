package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProblemContentType(t *testing.T) {
	recorder := httptest.NewRecorder()
	problem(recorder, http.StatusBadRequest, "invalid_request", "bad request")
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("unexpected content type: %s", got)
	}
}
