package proxy

import "testing"

func TestCipherRoundTrip(t *testing.T) {
	cipher, err := NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.Encrypt("secret", []byte("context"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := cipher.Decrypt(sealed, []byte("context"))
	if err != nil || plain != "secret" {
		t.Fatalf("round trip failed: %q %v", plain, err)
	}
	if _, err := cipher.Decrypt(sealed, []byte("other")); err == nil {
		t.Fatal("AAD mismatch accepted")
	}
}
