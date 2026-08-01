package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("a sufficiently long password", hash) {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("invalid password accepted")
	}
}

func TestPasswordValidation(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
	for _, value := range []string{"", "$argon2id$v=19$m=999999999,t=3,p=2$YQ$YQ", "$argon2i$v=19$m=1,t=1,p=1$YQ$YQ"} {
		if VerifyPassword("password", value) {
			t.Fatalf("invalid hash accepted: %q", value)
		}
	}
}
