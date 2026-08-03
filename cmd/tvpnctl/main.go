package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/FUjr/tvpn/sdk/go/tvpn"
)

type headers []string

func (h *headers) String() string         { return strings.Join(*h, ", ") }
func (h *headers) Set(value string) error { *h = append(*h, value); return nil }

func main() {
	if len(os.Args) < 2 || os.Args[1] != "request" {
		usage()
		os.Exit(2)
	}
	flags := flag.NewFlagSet("request", flag.ExitOnError)
	server := flags.String("server", os.Getenv("TVPN_SERVER"), "Tvpn management origin")
	token := flags.String("token", os.Getenv("TVPN_TOKEN"), "Tvpn program token")
	upstream := flags.String("upstream", "", "upstream proxy UUID; empty uses direct access")
	data := flags.String("data", "", "request body, @file, or @- for stdin")
	var requestHeaders headers
	flags.Var(&requestHeaders, "header", "target request header; repeatable")
	_ = flags.Parse(os.Args[2:])
	if *server == "" || *token == "" || flags.NArg() != 2 {
		flags.Usage()
		os.Exit(2)
	}
	body, closeBody, err := openBody(*data)
	if err != nil {
		fatal(err)
	}
	if closeBody != nil {
		defer closeBody()
	}
	header := make(http.Header)
	for _, value := range requestHeaders {
		name, content, ok := strings.Cut(value, ":")
		if !ok || strings.TrimSpace(name) == "" {
			fatal(fmt.Errorf("invalid header %q", value))
		}
		header.Add(strings.TrimSpace(name), strings.TrimSpace(content))
	}
	client := tvpn.NewClient(*server, *token)
	response, err := client.Do(context.Background(), tvpn.Request{Method: strings.ToUpper(flags.Arg(0)), URL: flags.Arg(1), Header: header, Body: body, UpstreamProxyID: *upstream})
	if err != nil {
		fatal(err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(os.Stdout, response.Body); err != nil {
		fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		os.Exit(1)
	}
}

func openBody(value string) (io.Reader, func() error, error) {
	if value == "" {
		return nil, nil, nil
	}
	if value == "@-" {
		return os.Stdin, nil, nil
	}
	if strings.HasPrefix(value, "@") {
		file, err := os.Open(strings.TrimPrefix(value, "@"))
		if err != nil {
			return nil, nil, err
		}
		return file, file.Close, nil
	}
	return strings.NewReader(value), nil, nil
}

func usage()          { fmt.Fprintln(os.Stderr, "usage: tvpnctl request [flags] METHOD URL") }
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
