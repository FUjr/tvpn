(() => {
  'use strict';
  const config = window.__TVPN_CONFIG__;
  if (!config || window.__TVPN_RUNTIME__) return;
  window.__TVPN_RUNTIME__ = true;

  const NativeFetch = window.fetch.bind(window);
  const NativeWebSocket = window.WebSocket;
  const NativeEventSource = window.EventSource;
  const nativeOpen = window.open.bind(window);
  const upstreamBase = new URL(config.upstreamURL);
  const INTERNAL_PREFIX = '/__tvpn/';

  const visibleCookies = new Map();
  for (const item of (config.cookies || '').split(/;\s*/)) { const index=item.indexOf('='); if(index>0)visibleCookies.set(item.slice(0,index),item.slice(index+1)); }
  function updateVisibleCookie(value) {
    const first = String(value).split(';', 1)[0], index = first.indexOf('=');
    if (index <= 0) return;
    const name=first.slice(0,index).trim(), cookieValue=first.slice(index+1).trim();
    if (/max-age\s*=\s*0/i.test(value) || cookieValue === '') visibleCookies.delete(name); else visibleCookies.set(name,cookieValue);
  }
  try {
    Object.defineProperty(Document.prototype, 'cookie', {
      configurable: true,
      get() { return [...visibleCookies].map(([name,value]) => `${name}=${value}`).join('; '); },
      set(value) {
		updateVisibleCookie(value);
        NativeFetch('/__tvpn/cookie', { method:'POST', credentials:'include', headers:{'Content-Type':'application/json','X-Tvpn-Upstream-Path':upstreamBase.pathname}, body:JSON.stringify({cookie:String(value)}) }).catch(()=>{});
      },
    });
  } catch {}

  function absoluteTarget(value) {
    const candidate = value instanceof Request ? value.url : String(value);
    const parsed = new URL(candidate, upstreamBase);
	if (parsed.host === location.host) {
	  const websocket = parsed.protocol === 'ws:' || parsed.protocol === 'wss:';
	  parsed.protocol = websocket ? (upstreamBase.protocol === 'https:' ? 'wss:' : 'ws:') : upstreamBase.protocol;
      parsed.host = upstreamBase.host;
    }
    return parsed.href;
  }

  async function resolveURL(value) {
    const response = await NativeFetch('/__tvpn/resolve', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: absoluteTarget(value) }),
    });
    if (!response.ok) throw new Error(`Tvpn URL resolution failed (${response.status})`);
    return (await response.json()).url;
  }

  const encoder = new TextEncoder();
  const decoder = new TextDecoder();
  const START = 1, CHUNK = 2, END = 3, CANCEL = 4;
  const RESPONSE = 11, RESPONSE_CHUNK = 12, RESPONSE_END = 13, RESPONSE_ERROR = 14;
  let nextID = 1;
  const pending = new Map();
  let socketPromise;

  function frame(type, id, payload = new Uint8Array()) {
    const body = payload instanceof Uint8Array ? payload : new Uint8Array(payload);
    const result = new Uint8Array(6 + body.byteLength);
    result[0] = 1; result[1] = type;
    new DataView(result.buffer).setUint32(2, id);
    result.set(body, 6);
    return result;
  }

  function muxSocket() {
    if (socketPromise) return socketPromise;
    socketPromise = new Promise((resolve, reject) => {
      const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const socket = new NativeWebSocket(`${scheme}//${location.host}/__tvpn/mux`);
      socket.binaryType = 'arraybuffer';
      socket.onopen = () => resolve(socket);
      socket.onerror = () => reject(new TypeError('Tvpn WebSocket transport unavailable'));
      socket.onclose = () => {
        for (const request of pending.values()) request.fail(new TypeError('Tvpn WebSocket transport closed'));
        pending.clear(); socketPromise = undefined;
      };
      socket.onmessage = event => handleFrame(new Uint8Array(event.data));
    });
    return socketPromise;
  }

  function handleFrame(data) {
    if (data.byteLength < 6 || data[0] !== 1) return;
    const type = data[1], id = new DataView(data.buffer, data.byteOffset, data.byteLength).getUint32(2);
    const request = pending.get(id); if (!request) return;
    const payload = data.subarray(6);
    if (type === RESPONSE) request.start(JSON.parse(decoder.decode(payload)));
    else if (type === RESPONSE_CHUNK) request.chunk(payload.slice());
    else if (type === RESPONSE_END) { request.end(); pending.delete(id); }
    else if (type === RESPONSE_ERROR) { request.fail(new TypeError(decoder.decode(payload))); pending.delete(id); }
  }

  async function muxFetch(input, init) {
    const target = absoluteTarget(input);
    if (new URL(target).pathname.startsWith(INTERNAL_PREFIX) && new URL(target).origin === upstreamBase.origin) return NativeFetch(input, init);
    const request = new Request(target, input instanceof Request && init === undefined ? input : init);
    const socket = await muxSocket();
    const id = nextID++ >>> 0 || nextID++;
    let controller;
    let responseResolve, responseReject;
    const responsePromise = new Promise((resolve, reject) => { responseResolve = resolve; responseReject = reject; });
    pending.set(id, {
      start(meta) {
		for (const cookie of (meta.cookies || [])) updateVisibleCookie(cookie);
        const hasBody = ![101, 204, 205, 304].includes(meta.status);
        const stream = hasBody ? new ReadableStream({ start(value) { controller = value; }, cancel() { if (socket.readyState === NativeWebSocket.OPEN) socket.send(frame(CANCEL, id)); } }) : null;
        responseResolve(new Response(stream, { status: meta.status, statusText: meta.statusText, headers: meta.headers }));
      },
      chunk(value) { controller?.enqueue(value); },
      end() { controller?.close(); },
      fail(error) { if (controller) controller.error(error); else responseReject(error); },
    });
    const headers = {}; request.headers.forEach((value, key) => { headers[key] = value; });
    socket.send(frame(START, id, encoder.encode(JSON.stringify({ method: request.method, url: target, headers }))));
    if (request.body) {
      const reader = request.body.getReader();
      while (true) { const { value, done } = await reader.read(); if (done) break; socket.send(frame(CHUNK, id, value)); }
    }
    socket.send(frame(END, id));
    return responsePromise;
  }

  window.fetch = muxFetch;

  class TvpnXHR extends EventTarget {
    constructor() { super(); this.readyState = 0; this.status = 0; this.statusText = ''; this.responseType = ''; this.response = null; this.responseText = ''; this.timeout = 0; this.withCredentials = true; this.headers = new Headers(); this.responseHeaders = new Headers(); this.controller = null; }
    open(method, url, async = true) { if (!async) throw new DOMException('Synchronous XHR is not supported by Tvpn', 'NotSupportedError'); this.method = method; this.url = url; this.readyState = 1; this.emit('readystatechange'); }
    setRequestHeader(name, value) { this.headers.append(name, value); }
    getResponseHeader(name) { return this.responseHeaders.get(name); }
    getAllResponseHeaders() { return [...this.responseHeaders].map(([k,v]) => `${k}: ${v}\r\n`).join(''); }
    overrideMimeType() {}
    abort() { this.controller?.abort(); this.emit('abort'); this.emit('loadend'); }
    async send(body = null) {
      this.controller = new AbortController();
      let timer;
      if (this.timeout > 0) timer = setTimeout(() => { this.controller.abort(); this.emit('timeout'); }, this.timeout);
      try {
        const response = await muxFetch(this.url, { method: this.method, headers: this.headers, body, signal: this.controller.signal });
        this.status = response.status; this.statusText = response.statusText; this.responseHeaders = response.headers; this.readyState = 2; this.emit('readystatechange');
        const type = this.responseType;
        if (type === 'arraybuffer') this.response = await response.arrayBuffer();
        else if (type === 'blob') this.response = await response.blob();
        else { this.responseText = await response.text(); this.response = type === 'json' ? JSON.parse(this.responseText || 'null') : this.responseText; }
        this.readyState = 4; this.emit('readystatechange'); this.emit('load');
      } catch (error) { if (error?.name !== 'AbortError') this.emit('error'); }
      finally { clearTimeout(timer); this.emit('loadend'); }
    }
    emit(type) { const event = new Event(type); this.dispatchEvent(event); const handler = this[`on${type}`]; if (typeof handler === 'function') handler.call(this, event); }
  }
  TvpnXHR.UNSENT = 0; TvpnXHR.OPENED = 1; TvpnXHR.HEADERS_RECEIVED = 2; TvpnXHR.LOADING = 3; TvpnXHR.DONE = 4;
  window.XMLHttpRequest = TvpnXHR;

  class TvpnEventSource extends EventTarget {
    constructor(url, options = {}) { super(); this.url = absoluteTarget(url); this.withCredentials = Boolean(options.withCredentials); this.readyState = 0; this.abort = new AbortController(); this.connect(); }
    async connect() { try { const response = await muxFetch(this.url, { headers: { Accept: 'text/event-stream' }, signal: this.abort.signal }); this.readyState = 1; this.emit('open', new Event('open')); const reader = response.body.getReader(); let buffer = ''; while (true) { const {value,done}=await reader.read(); if(done)break; buffer += decoder.decode(value,{stream:true}); let boundary; while((boundary=buffer.indexOf('\n\n'))>=0){const block=buffer.slice(0,boundary);buffer=buffer.slice(boundary+2);let event='message',data='';for(const line of block.split(/\r?\n/)){if(line.startsWith('event:'))event=line.slice(6).trim();if(line.startsWith('data:'))data+=line.slice(5).trimStart()+'\n';}this.emit(event,new MessageEvent(event,{data:data.replace(/\n$/,'')}));} } } catch { if(this.readyState!==2)this.emit('error',new Event('error')); } }
    emit(type,event){this.dispatchEvent(event);const handler=this[`on${type}`];if(typeof handler==='function')handler.call(this,event)}
    close(){this.readyState=2;this.abort.abort()}
  }
  TvpnEventSource.CONNECTING=0;TvpnEventSource.OPEN=1;TvpnEventSource.CLOSED=2;
  if (NativeEventSource) window.EventSource = TvpnEventSource;

  navigator.sendBeacon = (url, data) => { muxFetch(url, { method: 'POST', body: data, keepalive: true }).catch(() => {}); return true; };
  if (navigator.serviceWorker) navigator.serviceWorker.register = () => Promise.reject(new DOMException('Upstream service workers are disabled by Tvpn', 'NotSupportedError'));

  window.WebSocket = function TvpnWebSocket(url, protocols) {
    const target = absoluteTarget(url).replace(/^http:/, 'ws:').replace(/^https:/, 'wss:');
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const tunnel = `${scheme}//${location.host}/__tvpn/ws?target=${encodeURIComponent(target)}`;
    return protocols === undefined ? new NativeWebSocket(tunnel) : new NativeWebSocket(tunnel, protocols);
  };
  Object.assign(window.WebSocket, { CONNECTING: 0, OPEN: 1, CLOSING: 2, CLOSED: 3 });
  window.WebSocket.prototype = NativeWebSocket.prototype;

  async function navigate(value, destination = window) { destination.location.href = await resolveURL(value); }
  document.addEventListener('click', event => { const link = event.target.closest?.('a[href]'); if (!link || event.defaultPrevented || event.button !== 0 || link.download) return; const target = link.getAttribute('target'); if (target && target !== '_self') return; event.preventDefault(); navigate(link.href).catch(console.error); }, true);
  const nativeSubmit = HTMLFormElement.prototype.submit;
  HTMLFormElement.prototype.submit = function() { resolveURL(this.action || upstreamBase.href).then(action => { this.action = action; nativeSubmit.call(this); }).catch(console.error); };
  window.open = function(url, target, features) { if (!url) return nativeOpen(url, target, features); const child = nativeOpen('about:blank', target, features); if (child) navigate(url, child).catch(() => child.close()); return child; };

  const nativePushState = history.pushState.bind(history), nativeReplaceState = history.replaceState.bind(history);
  function updateHistory(nativeMethod, state, unused, value) {
    if (value === undefined || value === null) return nativeMethod(state, unused, value);
    const target = new URL(absoluteTarget(value));
    if (target.origin !== upstreamBase.origin) { navigate(target.href).catch(console.error); return; }
    const local = target.pathname + target.search + target.hash;
    const result = nativeMethod(state, unused, local);
    window.parent?.postMessage({ type:'tvpn:navigation', url:target.href, title:document.title }, config.appOrigin);
    return result;
  }
  history.pushState = (state, unused, url) => updateHistory(nativePushState, state, unused, url);
  history.replaceState = (state, unused, url) => updateHistory(nativeReplaceState, state, unused, url);
  addEventListener('popstate', () => { const target=new URL(location.href);target.protocol=upstreamBase.protocol;target.host=upstreamBase.host;window.parent?.postMessage({type:'tvpn:navigation',url:target.href,title:document.title},config.appOrigin); });
  addEventListener('message', event => {
    if (event.origin !== config.appOrigin || event.source !== window.parent || event.data?.type !== 'tvpn:command') return;
    if (event.data.action === 'back') history.back();
    if (event.data.action === 'forward') history.forward();
    if (event.data.action === 'reload') location.reload();
  });

  function rewriteNode(node) {
    if (!(node instanceof Element)) return;
    const names = ['href','src','action','formaction','poster','data','cite','background'];
    for (const name of names) if (node.hasAttribute(name)) { const raw=node.getAttribute(name); if(raw && !raw.startsWith('#') && !/^(data|blob|javascript|mailto|tel):/i.test(raw)) resolveURL(raw).then(value => { if (value !== raw) node.setAttribute(name,value); }).catch(()=>{}); }
  }
  new MutationObserver(records => { for (const record of records) { if(record.type==='attributes')rewriteNode(record.target);for(const node of record.addedNodes){rewriteNode(node);node.querySelectorAll?.('[href],[src],[action],[formaction],[poster],[data],[cite],[background]').forEach(rewriteNode);} } }).observe(document.documentElement,{subtree:true,childList:true,attributes:true,attributeFilter:['href','src','action','formaction','poster','data','cite','background']});

  if (window.navigation?.addEventListener) window.navigation.addEventListener('navigate', event => { if (!event.canIntercept || event.hashChange || event.downloadRequest) return; const destination = event.destination.url; if (new URL(destination).origin === location.origin) return; event.intercept({ handler: () => navigate(destination) }); });
  window.parent?.postMessage({ type: 'tvpn:navigation', url: upstreamBase.href, title: document.title }, config.appOrigin);
})();
