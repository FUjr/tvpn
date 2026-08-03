package auth

import "testing"

func TestValidateScopes(t *testing.T) {
	values, err := ValidateScopes([]string{ScopeProxy, ScopeProxy}, false)
	if err != nil || len(values) != 1 || values[0] != ScopeProxy {
		t.Fatalf("unexpected proxy scopes: %#v, %v", values, err)
	}
	if _, err := ValidateScopes([]string{ScopeAdmin}, false); err == nil {
		t.Fatal("non-administrator received admin scope")
	}
	if _, err := ValidateScopes([]string{"unknown"}, true); err == nil {
		t.Fatal("unknown scope was accepted")
	}
}
