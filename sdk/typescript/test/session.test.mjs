import assert from 'node:assert/strict'
import { TvpnClient } from '../dist/index.js'

const calls = []
const fakeFetch = async (url, init = {}) => {
  calls.push({ url: String(url), method: init.method || 'GET', headers: new Headers(init.headers) })
  if (String(url).endsWith('/api/v1/proxy/contexts/')) {
    return Response.json({ context: { id: 'persistent' }, route_url: 'https://route-one.proxy.example.com/login' })
  }
  if (String(url).endsWith('/navigate')) {
    return Response.json({ route_url: 'https://route-two.proxy.example.com/devices' })
  }
  if (String(url).endsWith('/api/v1/proxy/contexts/persistent')) {
    return new Response(null, { status: 204 })
  }
  return Response.json({ ok: true })
}

const client = new TvpnClient({ baseURL: 'https://vpn.example.com', token: 'tvpn_pat_test', fetch: fakeFetch })
const session = await client.open('https://api.example.com/login')
await session.fetch('https://api.example.com/login', { method: 'POST', json: { user: 'test' } })
await session.fetch('https://api.example.com/devices')
await session.close()

assert.equal(calls.filter(call => call.url.endsWith('/navigate')).length, 1)
assert.equal(calls.filter(call => call.headers.get('Proxy-Authorization') === 'Bearer tvpn_pat_test').length, 2)
assert.equal(calls.at(-1).method, 'DELETE')
