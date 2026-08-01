export type User = {
  id: string
  username: string
  display_name: string
  email: string
  auth_source: 'local' | 'ldap'
  is_admin: boolean
  disabled_at?: string | null
  last_login_at?: string | null
  policy_ids?: string[]
}

export type Session = { user: User; csrf_token: string; expires_at: string }
export type Rule = { id?: string; kind: 'domain' | 'cidr'; value: string }
export type Policy = { id: string; name: string; description: string; mode: PolicyMode; enabled: boolean; rules: Rule[] }
export type PolicyMode = 'deny_all' | 'deny_intranet' | 'whitelist' | 'blacklist'
export type PolicyInput = Omit<Policy, 'id'>
export type ProxyContext = { id: string; current_url: string; created_at: string; last_active_at: string }
export type Navigation = { context?: ProxyContext; bootstrap_url: string; route_url: string }
export type LDAPSettings = {
  enabled: boolean; mode: string; url: string; start_tls: boolean; base_dn: string; bind_dn: string
  user_filter: string; user_dn_template: string; username_attribute: string; display_name_attribute: string
  email_attribute: string; group_mode: string; group_base_dn: string; group_filter: string; group_name_attribute: string
}
export type LDAPGroup = { id: string; external_dn: string; name: string; last_seen_at: string; policy_ids: string[] }
export type AuditEvent = { id: number; actor_user_id?: string | null; event_type: string; outcome: string; target: string; detail: unknown; created_at: string }

let csrfToken = ''

export class APIError extends Error {
  constructor(public status: number, message: string) { super(message) }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  if (init.method && !['GET', 'HEAD'].includes(init.method)) headers.set('X-CSRF-Token', csrfToken)
  const response = await fetch(`/api/v1${path}`, { ...init, headers, credentials: 'same-origin' })
  if (!response.ok) {
    const problem = await response.json().catch(() => null) as { detail?: string } | null
    throw new APIError(response.status, problem?.detail || `请求失败 (${response.status})`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

function remember(session: Session) { csrfToken = session.csrf_token; return session }
const json = (value: unknown) => JSON.stringify(value)

export const api = {
  session: () => request<Session>('/auth/session').then(remember),
  login: (username: string, password: string) => request<Session>('/auth/login', { method: 'POST', body: json({ username, password }) }).then(remember),
  logout: () => request<void>('/auth/logout', { method: 'POST' }).finally(() => { csrfToken = '' }),
  createContext: (url: string) => request<Navigation>('/proxy/contexts/', { method: 'POST', body: json({ url }) }),
  navigate: (id: string, url: string) => request<Navigation>(`/proxy/contexts/${id}/navigate`, { method: 'POST', body: json({ url }) }),
  closeContext: (id: string) => request<void>(`/proxy/contexts/${id}`, { method: 'DELETE' }),
  users: () => request<{ items: User[] }>('/admin/users'),
  createUser: (value: { username: string; display_name: string; email: string; password: string; is_admin: boolean }) => request<User>('/admin/users', { method: 'POST', body: json(value) }),
  updateUser: (id: string, value: { is_admin?: boolean; disabled?: boolean }) => request<void>(`/admin/users/${id}`, { method: 'PATCH', body: json(value) }),
  resetPassword: (id: string, password: string) => request<void>(`/admin/users/${id}/password`, { method: 'POST', body: json({ password }) }),
  setUserPolicies: (id: string, policy_ids: string[]) => request<void>(`/admin/users/${id}/policies`, { method: 'PUT', body: json({ policy_ids }) }),
  policies: () => request<{ items: Policy[] }>('/admin/policies'),
  createPolicy: (value: PolicyInput) => request<{ id: string }>('/admin/policies', { method: 'POST', body: json(value) }),
  updatePolicy: (id: string, value: PolicyInput) => request<void>(`/admin/policies/${id}`, { method: 'PUT', body: json(value) }),
  deletePolicy: (id: string) => request<void>(`/admin/policies/${id}`, { method: 'DELETE' }),
  ldap: () => request<{ settings: LDAPSettings; bind_password_configured: boolean }>('/admin/ldap'),
  updateLDAP: (value: LDAPSettings) => request<void>('/admin/ldap', { method: 'PUT', body: json(value) }),
  testLDAP: () => request<void>('/admin/ldap/test', { method: 'POST' }),
  groups: () => request<{ items: LDAPGroup[] }>('/admin/ldap/groups'),
  setGroupPolicies: (id: string, policy_ids: string[]) => request<void>(`/admin/ldap/groups/${id}/policies`, { method: 'PUT', body: json({ policy_ids }) }),
  audit: () => request<{ items: AuditEvent[] }>('/admin/audit'),
}
