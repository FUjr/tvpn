// Package tvpn provides a programmatic client for HTTP APIs reachable through Tvpn.
package tvpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type Request struct {
	Method            string
	URL               string
	Header            http.Header
	Body              io.Reader
	UpstreamProxyID   string
	CompatibilityMode bool
}

type SessionOptions struct {
	UpstreamProxyID   string
	CompatibilityMode bool
}

type Session struct {
	client       *Client
	ID           string
	initialURL   string
	initialRoute string
	usedInitial  bool
	closed       bool
	mu           sync.Mutex
}

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	MessageID string `json:"message_id"`
	Message   string `json:"message"`
}

func (p *Problem) Error() string {
	return fmt.Sprintf("tvpn: %s (%s, HTTP %d)", p.Message, p.Code, p.Status)
}

type navigation struct {
	Context struct {
		ID string `json:"id"`
	} `json:"context"`
	RouteURL string `json:"route_url"`
}

func NewClient(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTPClient: http.DefaultClient}
}

func (c *Client) Do(ctx context.Context, input Request) (*http.Response, error) {
	if input.Method == "" {
		input.Method = http.MethodGet
	}
	navigation, err := c.createContext(ctx, input)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, navigation.RouteURL, input.Body)
	if err != nil {
		_ = c.closeContext(context.Background(), navigation.Context.ID)
		return nil, err
	}
	request.Header = cloneHeader(input.Header)
	request.Header.Set("Proxy-Authorization", "Bearer "+c.Token)
	client := *c.httpClient()
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		req.Header.Set("Proxy-Authorization", "Bearer "+c.Token)
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		_ = c.closeContext(context.Background(), navigation.Context.ID)
		return nil, err
	}
	if isProblem(response) {
		problem := decodeProblem(response)
		_ = response.Body.Close()
		_ = c.closeContext(context.Background(), navigation.Context.ID)
		return nil, problem
	}
	response.Body = &cleanupBody{ReadCloser: response.Body, cleanup: func() { _ = c.closeContext(context.Background(), navigation.Context.ID) }}
	return response, nil
}

// Open creates a persistent Tvpn context. Calls made through the returned
// session share the encrypted upstream Cookie Jar and selected egress.
func (c *Client) Open(ctx context.Context, targetURL string, options SessionOptions) (*Session, error) {
	navigation, err := c.createContext(ctx, Request{URL: targetURL, UpstreamProxyID: options.UpstreamProxyID, CompatibilityMode: options.CompatibilityMode})
	if err != nil {
		return nil, err
	}
	return &Session{client: c, ID: navigation.Context.ID, initialURL: targetURL, initialRoute: navigation.RouteURL}, nil
}

func (s *Session) Do(ctx context.Context, input Request) (*http.Response, error) {
	if input.Method == "" {
		input.Method = http.MethodGet
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("tvpn: session is closed")
	}
	routeURL := ""
	if !s.usedInitial && input.URL == s.initialURL {
		routeURL = s.initialRoute
		s.usedInitial = true
	}
	s.mu.Unlock()
	if routeURL == "" {
		var err error
		routeURL, err = s.client.navigate(ctx, s.ID, input.URL)
		if err != nil {
			return nil, err
		}
	}
	return s.client.doRoute(ctx, routeURL, input)
}

func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	if err := s.client.closeContext(ctx, s.ID); err != nil {
		s.mu.Lock()
		s.closed = false
		s.mu.Unlock()
		return err
	}
	return nil
}

func (c *Client) doRoute(ctx context.Context, routeURL string, input Request) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, input.Method, routeURL, input.Body)
	if err != nil {
		return nil, err
	}
	request.Header = cloneHeader(input.Header)
	request.Header.Set("Proxy-Authorization", "Bearer "+c.Token)
	client := *c.httpClient()
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		req.Header.Set("Proxy-Authorization", "Bearer "+c.Token)
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if isProblem(response) {
		problem := decodeProblem(response)
		_ = response.Body.Close()
		return nil, problem
	}
	return response, nil
}

func (c *Client) navigate(ctx context.Context, id, targetURL string) (string, error) {
	body, err := json.Marshal(map[string]string{"url": targetURL})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/proxy/contexts/"+url.PathEscape(id)+"/navigate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", decodeProblem(response)
	}
	var value struct {
		RouteURL string `json:"route_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return "", err
	}
	return value.RouteURL, nil
}

func (c *Client) createContext(ctx context.Context, input Request) (navigation, error) {
	body, err := json.Marshal(map[string]any{"url": input.URL, "upstream_proxy_id": nullable(input.UpstreamProxyID), "compatibility_mode": input.CompatibilityMode})
	if err != nil {
		return navigation{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/proxy/contexts/", bytes.NewReader(body))
	if err != nil {
		return navigation{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return navigation{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return navigation{}, decodeProblem(response)
	}
	var value navigation
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return navigation{}, err
	}
	if value.Context.ID == "" || value.RouteURL == "" {
		return navigation{}, fmt.Errorf("tvpn: incomplete context response")
	}
	return value, nil
}

func (c *Client) closeContext(ctx context.Context, id string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/api/v1/proxy/contexts/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	response, err := c.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeProblem(response)
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func cloneHeader(value http.Header) http.Header {
	if value == nil {
		return make(http.Header)
	}
	return value.Clone()
}

func isProblem(response *http.Response) bool {
	return (response.StatusCode < 200 || response.StatusCode >= 300) && response.Header.Get("Tvpn-Error-Code") != ""
}

func decodeProblem(response *http.Response) error {
	var problem Problem
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&problem); err != nil {
		return fmt.Errorf("tvpn: HTTP %d", response.StatusCode)
	}
	if problem.Status == 0 {
		problem.Status = response.StatusCode
	}
	return &problem
}

type cleanupBody struct {
	io.ReadCloser
	cleanup func()
	done    bool
}

func (b *cleanupBody) Close() error {
	err := b.ReadCloser.Close()
	if !b.done {
		b.done = true
		b.cleanup()
	}
	return err
}
