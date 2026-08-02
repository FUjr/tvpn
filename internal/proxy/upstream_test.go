package proxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestUpstreamPasswordEncryption(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	cipherValue, err := NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	store := &UpstreamStore{cipher: cipherValue}
	value := &Upstream{ID: uuid.New(), Username: "alice"}
	value.passwordEncrypted, err = cipherValue.Encrypt("secret", upstreamAAD(value.ID))
	if err != nil {
		t.Fatal(err)
	}
	password, err := store.Password(value)
	if err != nil || password != "secret" {
		t.Fatalf("Password() = %q, %v", password, err)
	}
	proxyURL, err := store.ProxyURL(&Upstream{ID: value.ID, Type: UpstreamHTTP, Host: "proxy.example.com", Port: 3128, Username: value.Username, passwordEncrypted: value.passwordEncrypted})
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL.Redacted() != "http://alice:xxxxx@proxy.example.com:3128" {
		t.Fatalf("unexpected redacted proxy URL: %s", proxyURL.Redacted())
	}
}

func TestNormalizeUpstreamInput(t *testing.T) {
	valid := UpstreamInput{Name: " Office ", Type: UpstreamSOCKS5, Host: "proxy.example.com.", Port: 1080}
	if err := normalizeUpstreamInput(&valid); err != nil {
		t.Fatal(err)
	}
	if valid.Name != "Office" || valid.Host != "proxy.example.com" {
		t.Fatalf("input was not normalized: %#v", valid)
	}
	invalid := UpstreamInput{Name: "bad", Type: UpstreamHTTP, Host: "http://proxy.example.com", Port: 3128}
	if err := normalizeUpstreamInput(&invalid); err == nil {
		t.Fatal("expected a URL-shaped host to be rejected")
	}
}

func TestHTTPProxyConnectUsesPinnedTargetAndAuthentication(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan string, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			result <- acceptErr.Error()
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		var lines []string
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				result <- readErr.Error()
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			lines = append(lines, line)
		}
		result <- strings.Join(lines, "\n")
		_, _ = connection.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	}()
	proxyURL, _ := url.Parse("http://" + listener.Addr().String())
	proxyURL.User = url.UserPassword("alice", "secret")
	connection, err := dialHTTPProxy(context.Background(), &net.Dialer{}, proxyURL, "tcp", "203.0.113.9:8080")
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	got := <-result
	if !strings.Contains(got, "CONNECT 203.0.113.9:8080 HTTP/1.1") || !strings.Contains(got, "Proxy-Authorization: Basic YWxpY2U6c2VjcmV0") {
		t.Fatalf("unexpected CONNECT request:\n%s", got)
	}
}
