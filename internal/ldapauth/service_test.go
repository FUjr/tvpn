package ldapauth

import "testing"

func TestTemplateEscaping(t *testing.T) {
	if got := renderFilter("(uid={{username}})", "username", "a*)(uid=*)"); got != "(uid=a\\2a\\29\\28uid=\\2a\\29)" {
		t.Fatalf("filter was not escaped: %s", got)
	}
	if got := renderDN("uid={{username}},dc=example", "a,b"); got != "uid=a\\,b,dc=example" {
		t.Fatalf("DN was not escaped: %s", got)
	}
}
