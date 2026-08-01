package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/coder/websocket"
)

func main() {
	listen := flag.String("listen", ":18999", "fixture listen address")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/", page)
	mux.HandleFunc("/api", api)
	mux.HandleFunc("/cookie-check", func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("fixture"); err == nil && cookie.Value == "present" {
			_, _ = fmt.Fprint(w, "cookie-ok")
			return
		}
		http.Error(w, "missing cookie", http.StatusUnauthorized)
	})
	mux.HandleFunc("/xhr", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "xhr-ok") })
	mux.HandleFunc("/events", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: sse-ok\n\n")
	})
	mux.HandleFunc("/cookie-js-check", func(w http.ResponseWriter, r *http.Request) {
		visible, _ := r.Cookie("visible")
		client, _ := r.Cookie("client")
		if visible != nil && visible.Value == "server" && client != nil && client.Value == "browser" {
			_, _ = fmt.Fprint(w, "document-cookie-ok")
			return
		}
		http.Error(w, "missing JavaScript cookie", http.StatusUnauthorized)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) })
	mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "redirect-ok") })
	mux.HandleFunc("/ws", echo)
	log.Printf("Tvpn fixture listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}

func page(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	_, _ = fmt.Fprint(w, `<!doctype html><html><head><title>Tvpn fixture</title></head><body>
<a id="redirect" href="/redirect">redirect</a><div id="fetch">pending</div><div id="cookie">pending</div><div id="cookie-js">pending</div><div id="xhr">pending</div><div id="sse">pending</div><div id="socket">pending</div><div id="binary">pending</div>
<script>
fetch('/api').then(r => r.json()).then(v => { document.querySelector('#fetch').textContent = v.value; document.cookie = 'client=browser; Path=/'; return fetch('/cookie-check'); }).then(r => r.text()).then(v => { document.querySelector('#cookie').textContent = v; setTimeout(() => fetch('/cookie-js-check').then(r => r.text()).then(text => document.querySelector('#cookie-js').textContent = text), 100); });
const xhr = new XMLHttpRequest(); xhr.open('GET', '/xhr'); xhr.onload = () => document.querySelector('#xhr').textContent = xhr.responseText; xhr.send();
const events = new EventSource('/events'); events.onmessage = event => { document.querySelector('#sse').textContent = event.data; events.close(); };
const socket = new WebSocket('ws://' + location.host + '/ws', ['tvpn-echo']);
socket.onopen = () => socket.send('hello');
socket.onmessage = event => { document.querySelector('#socket').textContent = event.data + ':' + socket.protocol; const binary = new WebSocket('ws://' + location.host + '/ws'); binary.binaryType = 'arraybuffer'; binary.onopen = () => binary.send(new Uint8Array([1,2,3])); binary.onmessage = value => document.querySelector('#binary').textContent = Array.from(new Uint8Array(value.data)).join(','); };
</script></body></html>`)
}

func api(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "fixture", Value: "present", Path: "/", HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "visible", Value: "server", Path: "/"})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"value": "fetch-ok"})
}

func echo(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"tvpn-echo"}})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	for {
		messageType, value, err := conn.Read(context.Background())
		if err != nil {
			return
		}
		if err := conn.Write(context.Background(), messageType, append([]byte("echo-"), value...)); err != nil {
			return
		}
	}
}
