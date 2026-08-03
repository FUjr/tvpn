export type RequestOptions = {
  method?: string
  headers?: HeadersInit
  body?: BodyInit | null
  json?: unknown
  upstreamProxyId?: string
  compatibilityMode?: boolean
}

export class TvpnProblem extends Error {
  constructor(
    public status: number,
    public code: string,
    public messageId: string,
    message: string,
  ) { super(message) }
}

type Navigation = { context: { id: string }; route_url: string }

export class TvpnClient {
  private baseURL: string
  private token: string
  private fetchImpl: typeof fetch

  constructor(options: { baseURL: string; token: string; fetch?: typeof fetch }) {
    this.baseURL = options.baseURL.replace(/\/$/, '')
    this.token = options.token
    this.fetchImpl = options.fetch || globalThis.fetch
    if (!this.fetchImpl) throw new Error('A Fetch API implementation is required')
  }

  async fetch(url: string, options: RequestOptions = {}): Promise<Response> {
    const session = await this.open(url, options)
    try {
      return await session.fetch(url, options)
    } finally {
      await session.close().catch(() => undefined)
    }
  }

  async open(url: string, options: Pick<RequestOptions, 'upstreamProxyId' | 'compatibilityMode'> = {}): Promise<TvpnSession> {
    const navigation = await this.management<Navigation>('/api/v1/proxy/contexts/', {
      method: 'POST',
      body: JSON.stringify({
        url,
        upstream_proxy_id: options.upstreamProxyId || null,
        compatibility_mode: options.compatibilityMode || false,
      }),
      headers: { 'Content-Type': 'application/json' },
    })
    return new TvpnSession(this, navigation.context.id, url, navigation.route_url)
  }

  async route(routeURL: string, options: RequestOptions = {}): Promise<Response> {
    const headers = new Headers(options.headers)
    let body = options.body
    if (options.json !== undefined) {
      if (body != null) throw new Error('body and json are mutually exclusive')
      body = JSON.stringify(options.json)
      headers.set('Content-Type', 'application/json')
    }
    let method = (options.method || 'GET').toUpperCase()
    for (let redirects = 0; redirects <= 10; redirects += 1) {
      headers.set('Proxy-Authorization', `Bearer ${this.token}`)
      const response = await this.fetchImpl(routeURL, { method, headers, body, redirect: 'manual' })
      if ([301, 302, 303, 307, 308].includes(response.status) && response.headers.get('location')) {
        const nextURL = new URL(response.headers.get('location')!, routeURL).toString()
        if (new URL(nextURL).hostname !== new URL(routeURL).hostname) headers.delete('Authorization')
        routeURL = nextURL
        if (response.status === 303 || ((response.status === 301 || response.status === 302) && method === 'POST')) {
          method = 'GET'
          body = null
          headers.delete('Content-Type')
        }
        continue
      }
      if (response.status < 200 || response.status >= 300) {
        if (response.headers.get('Tvpn-Error-Code')) {
          throw await this.problem(response)
        }
      }
      const content = await response.arrayBuffer()
      return new Response(content, { status: response.status, statusText: response.statusText, headers: response.headers })
    }
    throw new Error('Tvpn stopped after 10 redirects')
  }

  async management<T = void>(path: string, init: RequestInit): Promise<T> {
    const headers = new Headers(init.headers)
    headers.set('Authorization', `Bearer ${this.token}`)
    const response = await this.fetchImpl(this.baseURL + path, { ...init, headers })
    if (!response.ok) throw await this.problem(response)
    if (response.status === 204) return undefined as T
    return await response.json() as T
  }

  private async problem(response: Response): Promise<TvpnProblem> {
    const value = await response.json().catch(() => ({})) as Record<string, unknown>
    return new TvpnProblem(
      Number(value.status || response.status),
      String(value.code || 'unknown_error'),
      String(value.message_id || 'error.common.request_failed'),
      String(value.message || `Tvpn returned HTTP ${response.status}`),
    )
  }
}

export class TvpnSession {
  private usedInitial = false
  private closed = false

  constructor(
    private client: TvpnClient,
    public readonly id: string,
    private initialURL: string,
    private initialRoute: string,
  ) {}

  async fetch(url: string, options: RequestOptions = {}): Promise<Response> {
    if (this.closed) throw new Error('Tvpn session is closed')
    let routeURL: string
    if (!this.usedInitial && url === this.initialURL) {
      this.usedInitial = true
      routeURL = this.initialRoute
    } else {
      const navigation = await this.client.management<Pick<Navigation, 'route_url'>>(`/api/v1/proxy/contexts/${encodeURIComponent(this.id)}/navigate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url }),
      })
      routeURL = navigation.route_url
    }
    return this.client.route(routeURL, options)
  }

  async close(): Promise<void> {
    if (!this.closed) {
      await this.client.management(`/api/v1/proxy/contexts/${encodeURIComponent(this.id)}`, { method: 'DELETE' })
      this.closed = true
    }
  }
}
