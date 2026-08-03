package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestProblemUsesStableFieldsAndAcceptLanguage(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	recorder := httptest.NewRecorder()
	Problem(recorder, request, ErrInvalidToken)
	if recorder.Header().Get("Tvpn-Error-Code") != string(ErrInvalidToken) {
		t.Fatalf("missing gateway error marker: %q", recorder.Header().Get("Tvpn-Error-Code"))
	}
	var value map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value["code"] != string(ErrInvalidToken) || value["message_id"] != "error.auth.invalid_token" || value["message"] != "Program token is invalid or expired" {
		t.Fatalf("unexpected problem: %#v", value)
	}
}

func TestEveryErrorHasStableDefinition(t *testing.T) {
	documentation, err := os.ReadFile("../../docs/error-codes.md")
	if err != nil {
		t.Fatal(err)
	}
	frontend, err := os.ReadFile("../../web/src/i18n.ts")
	if err != nil {
		t.Fatal(err)
	}
	for code, definition := range errorCatalog {
		if code == "" || definition.Status < 400 || definition.MessageID == "" || definition.ZH == "" || definition.EN == "" {
			t.Fatalf("incomplete error definition for %q: %#v", code, definition)
		}
		if !strings.Contains(string(documentation), "`"+string(code)+"`") || !strings.Contains(string(documentation), "`"+definition.MessageID+"`") {
			t.Fatalf("error %q is missing from docs/error-codes.md", code)
		}
		if strings.Count(string(frontend), "'"+definition.MessageID+"'") < 2 {
			t.Fatalf("error %q is missing a frontend translation", code)
		}
	}
}
