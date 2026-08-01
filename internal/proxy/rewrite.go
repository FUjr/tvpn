package proxy

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

//go:embed runtime.js
var runtimeJS []byte

const maxRewriteBody = 16 << 20

var cssURLPattern = regexp.MustCompile(`(?i)url\(\s*(['"]?)([^'"\)]+)['"]?\s*\)`)

func (s *Service) rewriteResponse(ctx context.Context, route Route, target *url.URL, contentType string, body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxRewriteBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxRewriteBody {
		return nil, errors.New("rewritable response exceeds 16 MiB")
	}
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return s.rewriteHTML(ctx, route, target, raw)
	}
	if strings.Contains(strings.ToLower(contentType), "text/css") {
		return []byte(s.rewriteCSS(ctx, route, target, string(raw))), nil
	}
	return raw, nil
}

func (s *Service) rewriteHTML(ctx context.Context, route Route, target *url.URL, raw []byte) ([]byte, error) {
	document, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	var head *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "head" && head == nil {
			head = node
		}
		if node.Type == html.ElementNode {
			s.rewriteElement(ctx, route, target, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if head == nil {
		return nil, errors.New("HTML document has no head")
	}
	var visibleCookies []*http.Cookie
	if s.store != nil {
		visibleCookies, err = s.store.VisibleCookies(ctx, s.cipher, route.ContextID, target)
		if err != nil {
			return nil, err
		}
	}
	cookieValues := make([]string, 0, len(visibleCookies))
	for _, cookie := range visibleCookies {
		cookieValues = append(cookieValues, cookie.Name+"="+cookie.Value)
	}
	config, _ := json.Marshal(map[string]string{"appOrigin": s.appOrigin.String(), "upstreamURL": target.String(), "contextID": route.ContextID.String(), "cookies": strings.Join(cookieValues, "; ")})
	configNode := &html.Node{Type: html.ElementNode, Data: "script"}
	configNode.AppendChild(&html.Node{Type: html.TextNode, Data: "window.__TVPN_CONFIG__=" + string(config) + ";"})
	runtimeNode := &html.Node{Type: html.ElementNode, Data: "script", Attr: []html.Attribute{{Key: "src", Val: "/__tvpn/runtime.js"}}}
	head.InsertBefore(runtimeNode, head.FirstChild)
	head.InsertBefore(configNode, runtimeNode)
	var result bytes.Buffer
	if err := html.Render(&result, document); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func (s *Service) rewriteElement(ctx context.Context, route Route, base *url.URL, node *html.Node) {
	attributes := map[string]bool{"href": true, "src": true, "action": true, "formaction": true, "poster": true, "data": true, "cite": true, "background": true, "manifest": true}
	filtered := node.Attr[:0]
	for i := range node.Attr {
		attribute := node.Attr[i]
		if attribute.Key == "integrity" {
			continue
		}
		if attributes[attribute.Key] {
			attribute.Val = s.rewriteURL(ctx, route, base, attribute.Val)
		} else if attribute.Key == "style" {
			attribute.Val = s.rewriteCSS(ctx, route, base, attribute.Val)
		} else if attribute.Key == "srcset" {
			parts := strings.Split(attribute.Val, ",")
			for j, part := range parts {
				fields := strings.Fields(strings.TrimSpace(part))
				if len(fields) > 0 {
					fields[0] = s.rewriteURL(ctx, route, base, fields[0])
					parts[j] = strings.Join(fields, " ")
				}
			}
			attribute.Val = strings.Join(parts, ", ")
		}
		filtered = append(filtered, attribute)
	}
	node.Attr = filtered
	if node.Data == "meta" {
		var equiv, content *html.Attribute
		for i := range node.Attr {
			if strings.EqualFold(node.Attr[i].Key, "http-equiv") {
				equiv = &node.Attr[i]
			}
			if node.Attr[i].Key == "content" {
				content = &node.Attr[i]
			}
		}
		if equiv != nil && content != nil && strings.EqualFold(equiv.Val, "refresh") {
			if index := strings.Index(strings.ToLower(content.Val), "url="); index >= 0 {
				content.Val = content.Val[:index+4] + s.rewriteURL(ctx, route, base, strings.TrimSpace(content.Val[index+4:]))
			}
		}
	}
}

func (s *Service) rewriteURL(ctx context.Context, route Route, base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || regexp.MustCompile(`(?i)^(data|blob|javascript|mailto|tel):`).MatchString(raw) {
		return raw
	}
	resolved, err := base.Parse(raw)
	if err != nil || !(resolved.Scheme == "http" || resolved.Scheme == "https") {
		return "about:blank"
	}
	fragment := resolved.Fragment
	resolved.Fragment = ""
	if strings.EqualFold(resolved.Scheme, base.Scheme) && strings.EqualFold(resolved.Host, base.Host) {
		value := resolved.EscapedPath() + querySuffix(resolved.RawQuery)
		if fragment != "" {
			value += "#" + url.PathEscape(fragment)
		}
		return value
	}
	mapped, err := s.store.ResolveRoute(ctx, route.ContextID, route.UserID, resolved)
	if err != nil {
		return "about:blank"
	}
	value := s.routeURL(mapped, resolved)
	if fragment != "" {
		value += "#" + url.PathEscape(fragment)
	}
	return value
}
func (s *Service) rewriteCSS(ctx context.Context, route Route, base *url.URL, value string) string {
	return cssURLPattern.ReplaceAllStringFunc(value, func(match string) string {
		sub := cssURLPattern.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		rewritten := s.rewriteURL(ctx, route, base, sub[2])
		return fmt.Sprintf("url(%s%s%s)", sub[1], rewritten, sub[1])
	})
}
