import { FormEvent, type ReactNode, useEffect, useRef, useState } from 'react'
import { ArrowLeft, ArrowRight, BookOpen, ChevronDown, ExternalLink, Globe2, KeyRound, LogOut, Network, PanelTop, Plus, RefreshCw, Save, Settings, ShieldCheck, Trash2, UserPlus, Users, Wifi } from 'lucide-react'
import { api, APIError, type AuditEvent, type DirectAccess, type LDAPGroup, type LDAPSettings, type Policy, type PolicyInput, type PolicyMode, type Session, type UpstreamProxy, type UpstreamProxyInput, type User } from './api/client'

type View = 'browser' | 'users' | 'policies' | 'upstreams' | 'ldap' | 'audit'
const policyLabels: Record<PolicyMode, string> = { deny_all: '禁止访问', deny_intranet: '禁止内网', whitelist: '白名单', blacklist: '黑名单' }
const blankPolicy: PolicyInput = { name: '', description: '', mode: 'deny_intranet', enabled: true, rules: [] }

export function App() {
  const [session, setSession] = useState<Session | null | undefined>()
  const [view, setView] = useState<View>('browser')
  const [notice, setNotice] = useState('')
  useEffect(() => { api.session().then(setSession).catch(() => setSession(null)) }, [])
  if (session === undefined) return <main className="boot"><span className="brand-mark">T</span></main>
  if (!session) return <Login onLogin={setSession} />
  const show = (message: string) => { setNotice(message); window.setTimeout(() => setNotice(''), 2600) }
  return <div className="app-shell">
    <header className="topbar">
      <button className="brand" onClick={() => setView('browser')}><span className="brand-mark">T</span><span>Tvpn</span></button>
      <nav className="nav-tabs" aria-label="主导航">
        <NavButton active={view === 'browser'} icon={<PanelTop />} label="浏览" onClick={() => setView('browser')} />
        {session.user.is_admin && <>
          <NavButton active={view === 'users'} icon={<Users />} label="用户" onClick={() => setView('users')} />
	          <NavButton active={view === 'policies'} icon={<ShieldCheck />} label="策略" onClick={() => setView('policies')} />
	          <NavButton active={view === 'upstreams'} icon={<Network />} label="代理" onClick={() => setView('upstreams')} />
          <NavButton active={view === 'ldap'} icon={<Settings />} label="LDAP" onClick={() => setView('ldap')} />
          <NavButton active={view === 'audit'} icon={<BookOpen />} label="审计" onClick={() => setView('audit')} />
        </>}
      </nav>
      <div className="account"><span>{session.user.display_name || session.user.username}</span><button className="icon-button" title="退出登录" onClick={() => api.logout().then(() => setSession(null))}><LogOut /></button></div>
    </header>
    <main className={view === 'browser' ? 'workspace' : 'admin-page'}>
      {view === 'browser' && <Browser />}
      {view === 'users' && <UsersView show={show} />}
	      {view === 'policies' && <PoliciesView show={show} />}
	      {view === 'upstreams' && <UpstreamsView show={show} />}
      {view === 'ldap' && <LDAPView show={show} />}
      {view === 'audit' && <AuditView />}
    </main>
    {notice && <div className="toast" role="status">{notice}</div>}
  </div>
}

function Login({ onLogin }: { onLogin: (session: Session) => void }) {
  const [username, setUsername] = useState(''); const [password, setPassword] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setError(''); try { onLogin(await api.login(username, password)) } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  return <main className="login-page"><section className="login-panel">
    <div className="login-brand"><span className="brand-mark">T</span><div><h1>Tvpn</h1><p>安全访问授权的 Web 资源</p></div></div>
    <form onSubmit={submit}><label>用户名<input autoFocus autoComplete="username" value={username} onChange={e => setUsername(e.target.value)} /></label><label>密码<input type="password" autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} /></label>{error && <p className="form-error">{error}</p>}<button className="primary" disabled={busy || !username || !password}><KeyRound />{busy ? '正在登录' : '登录'}</button></form>
    <p className="legal">Tvpn 按 AGPL-3.0 提供且不附带担保。<a href="https://github.com/FUjr/tvpn" target="_blank" rel="noreferrer">查看许可证与源代码</a></p>
  </section></main>
}

function Browser() {
  const [address, setAddress] = useState(''); const [contextID, setContextID] = useState(''); const [frameURL, setFrameURL] = useState(''); const [error, setError] = useState(''); const [loading, setLoading] = useState(false); const [upstreams, setUpstreams] = useState<UpstreamProxy[]>([]); const [directAllowed, setDirectAllowed] = useState<boolean>(); const [upstreamID, setUpstreamID] = useState(''); const frame = useRef<HTMLIFrameElement>(null)
  const go = async (event?: FormEvent) => { event?.preventDefault(); if (!address.trim()) return; setLoading(true); setError(''); try { const nav = contextID ? await api.navigate(contextID, address) : await api.createContext(address, upstreamID); if (nav.context) setContextID(nav.context.id); setFrameURL(nav.bootstrap_url) } catch (e) { setError(message(e)) } finally { setLoading(false) } }
  const command = (action: string) => frame.current?.contentWindow?.postMessage({ type: 'tvpn:command', action }, new URL(frameURL).origin)
  useEffect(() => { const receive = (event: MessageEvent) => { if (event.source !== frame.current?.contentWindow || event.data?.type !== 'tvpn:navigation') return; if (typeof event.data.url === 'string') setAddress(event.data.url) }; addEventListener('message', receive); return () => removeEventListener('message', receive) }, [])
  useEffect(() => { api.availableUpstreams().then(x => { setUpstreams(x.items); setDirectAllowed(x.direct_allowed); if (!x.direct_allowed && x.items.length) setUpstreamID(x.items[0].id) }).catch(e => setError(message(e))) }, [])
  useEffect(() => () => { if (contextID) void api.closeContext(contextID) }, [contextID])
  const noExit = directAllowed === false && upstreams.length === 0
  return <section className="browser-shell">
    <form className="browser-toolbar" onSubmit={go}>
      <div className="history-controls"><button type="button" className="icon-button" title="后退" disabled={!frameURL} onClick={() => command('back')}><ArrowLeft /></button><button type="button" className="icon-button" title="前进" disabled={!frameURL} onClick={() => command('forward')}><ArrowRight /></button><button type="button" className="icon-button" title="刷新" disabled={!frameURL} onClick={() => command('reload')}><RefreshCw className={loading ? 'spin' : ''} /></button></div>
      <label className="upstream-selector"><Network /><select aria-label="访问出口" title="访问出口" value={upstreamID} disabled={directAllowed === undefined || noExit} onChange={e => { if (contextID) void api.closeContext(contextID); setContextID(''); setFrameURL(''); setUpstreamID(e.target.value) }}>{directAllowed && <option value="">服务端直连</option>}{noExit && <option value="">无可用出口</option>}{upstreams.map(value => <option key={value.id} value={value.id}>{value.name} · {value.type.toUpperCase()}</option>)}</select></label>
      <div className="address-field"><Globe2 /><input aria-label="网址" placeholder="输入网址或 IP 地址" value={address} onChange={e => setAddress(e.target.value)} /><button className="go-button" title="访问" disabled={loading || directAllowed === undefined || noExit || !address.trim()}><ExternalLink /></button></div>
    </form>
    {error && <div className="browser-error">{error}</div>}
    <div className="viewport">{frameURL ? <iframe ref={frame} src={frameURL} title="WebVPN 页面" onLoad={() => setLoading(false)} /> : <div className="empty-browser"><Globe2 /><h1>开始安全浏览</h1><p>在上方输入已获授权的网址</p></div>}</div>
  </section>
}

function UpstreamsView({ show }: { show: (s: string) => void }) {
  const [items, setItems] = useState<UpstreamProxy[]>([]); const [direct, setDirect] = useState<DirectAccess>(); const [users, setUsers] = useState<User[]>([]); const [groups, setGroups] = useState<LDAPGroup[]>([]); const [editing, setEditing] = useState<UpstreamProxy | null | undefined>()
  const load = () => Promise.all([api.upstreams(), api.directAccess(), api.users(), api.groups()]).then(([p, d, u, g]) => { setItems(p.items); setDirect(d); setUsers(u.items); setGroups(g.items) })
  useEffect(() => { void load() }, [])
  return <Page title="访问出口" action={<button className="primary" onClick={() => setEditing(null)}><Plus />新建代理</button>}><div className="table-wrap"><table><thead><tr><th>出口</th><th>类型</th><th>地址</th><th>认证</th><th>状态</th><th>授权用户</th><th>授权 LDAP 组</th><th>操作</th></tr></thead><tbody>{direct && <tr><td><strong>服务端直连</strong><small>使用 Tvpn 服务器网络</small></td><td>DIRECT</td><td>本机网络</td><td>不适用</td><td><span className={`status ${direct.restricted ? 'warn' : ''}`}>{direct.restricted ? 'ACL 限制' : '所有用户'}</span></td><td><ResourcePicker label="用户" values={direct.user_ids} options={users.map(user => ({ id: user.id, name: user.display_name || user.username }))} onChange={async ids => { await api.setDirectAccessUsers(ids); await load(); show('直连用户授权已更新') }} /></td><td><ResourcePicker label="LDAP 组" values={direct.group_ids} options={groups.map(group => ({ id: group.id, name: group.name }))} onChange={async ids => { await api.setDirectAccessGroups(ids); await load(); show('直连组授权已更新') }} /></td><td><button className="quiet" onClick={async () => { await api.updateDirectAccess(!direct.restricted); await load(); show(direct.restricted ? '直连已允许所有用户' : '直连 ACL 已启用') }}>{direct.restricted ? '允许所有用户' : '启用 ACL'}</button></td></tr>}{items.map(value => <tr key={value.id}><td><strong>{value.name}</strong></td><td>{value.type.toUpperCase()}</td><td className="mono">{value.host}:{value.port}</td><td>{value.username ? `${value.username}${value.password_configured ? ' · 密码已配置' : ''}` : '无认证'}</td><td><span className={`status ${value.enabled ? '' : 'off'}`}>{value.enabled ? '启用' : '停用'}</span></td><td><ResourcePicker label="用户" values={value.user_ids || []} options={users.map(user => ({ id: user.id, name: user.display_name || user.username }))} onChange={async ids => { await api.setUpstreamUsers(value.id, ids); await load(); show('用户授权已更新') }} /></td><td><ResourcePicker label="LDAP 组" values={value.group_ids || []} options={groups.map(group => ({ id: group.id, name: group.name }))} onChange={async ids => { await api.setUpstreamGroups(value.id, ids); await load(); show('组授权已更新') }} /></td><td><div className="row-actions"><button className="quiet" onClick={() => setEditing(value)}>编辑</button><button className="icon-button danger" title="删除" onClick={async () => { if (confirm(`删除代理“${value.name}”？活动中的相关浏览上下文也会关闭。`)) { await api.deleteUpstream(value.id); await load(); show('代理已删除') } }}><Trash2 /></button></div></td></tr>)}</tbody></table></div>{editing !== undefined && <UpstreamDialog value={editing} close={() => setEditing(undefined)} done={async () => { setEditing(undefined); await load(); show('代理已保存') }} />}</Page>
}

function UpstreamDialog({ value, close, done }: { value: UpstreamProxy | null; close: () => void; done: () => void }) {
  const [input, setInput] = useState<UpstreamProxyInput>(value ? { name: value.name, type: value.type, host: value.host, port: value.port, username: value.username, password: '', enabled: value.enabled } : { name: '', type: 'http', host: '', port: 3128, username: '', password: '', enabled: true }); const [error, setError] = useState('')
  return <div className="modal-backdrop" onMouseDown={e => e.target === e.currentTarget && close()}><form className="modal" onSubmit={async e => { e.preventDefault(); try { if (value) await api.updateUpstream(value.id, input); else await api.createUpstream(input); done() } catch (x) { setError(message(x)) } }}><h2>{value ? '编辑上游代理' : '新建上游代理'}</h2><div className="form-grid"><label>名称<input required value={input.name} onChange={e => setInput({ ...input, name: e.target.value })} /></label><label>类型<select value={input.type} onChange={e => setInput({ ...input, type: e.target.value as 'http' | 'socks5', port: input.port || (e.target.value === 'socks5' ? 1080 : 3128) })}><option value="http">HTTP</option><option value="socks5">SOCKS5</option></select></label><label>主机或 IP<input required placeholder="proxy.example.com" value={input.host} onChange={e => setInput({ ...input, host: e.target.value })} /></label><label>端口<input required type="number" min="1" max="65535" value={input.port} onChange={e => setInput({ ...input, port: Number(e.target.value) })} /></label><label>用户名<input autoComplete="off" value={input.username} onChange={e => setInput({ ...input, username: e.target.value })} /></label><label>密码<input type="password" autoComplete="new-password" placeholder={value?.password_configured ? '留空则保留现有密码' : '可选'} value={input.password} onChange={e => setInput({ ...input, password: e.target.value })} /></label><label className="checkbox wide"><input type="checkbox" checked={input.enabled} onChange={e => setInput({ ...input, enabled: e.target.checked })} />启用代理</label>{value?.password_configured && <label className="checkbox wide"><input type="checkbox" checked={Boolean(input.clear_password)} onChange={e => setInput({ ...input, clear_password: e.target.checked })} />清除已保存的密码</label>}</div>{error && <p className="form-error">{error}</p>}<div className="modal-actions"><button type="button" className="quiet" onClick={close}>取消</button><button className="primary"><Save />保存</button></div></form></div>
}

function UsersView({ show }: { show: (s: string) => void }) {
  const [users, setUsers] = useState<User[]>([]); const [policies, setPolicies] = useState<Policy[]>([]); const [open, setOpen] = useState(false)
  const load = () => Promise.all([api.users(), api.policies()]).then(([u, p]) => { setUsers(u.items); setPolicies(p.items) })
  useEffect(() => { void load() }, [])
  return <Page title="用户" action={<button className="primary" onClick={() => setOpen(true)}><UserPlus />新建用户</button>}>
    <div className="table-wrap"><table><thead><tr><th>用户</th><th>来源</th><th>角色</th><th>状态</th><th>策略</th><th>操作</th></tr></thead><tbody>{users.map(user => <tr key={user.id}><td><strong>{user.display_name || user.username}</strong><small>{user.username}{user.email ? ` · ${user.email}` : ''}</small></td><td>{user.auth_source.toUpperCase()}</td><td>{user.is_admin ? '管理员' : '用户'}</td><td><span className={`status ${user.disabled_at ? 'off' : ''}`}>{user.disabled_at ? '已禁用' : '正常'}</span></td><td><PolicyPicker values={user.policy_ids || []} policies={policies} onChange={async ids => { await api.setUserPolicies(user.id, ids); await load(); show('用户策略已更新') }} /></td><td><button className="quiet" onClick={async () => { await api.updateUser(user.id, { disabled: !user.disabled_at }); await load() }}>{user.disabled_at ? '启用' : '禁用'}</button></td></tr>)}</tbody></table></div>
    {open && <UserDialog close={() => setOpen(false)} done={async () => { setOpen(false); await load(); show('用户已创建') }} />}
  </Page>
}

function UserDialog({ close, done }: { close: () => void; done: () => void }) {
  const [value, setValue] = useState({ username: '', display_name: '', email: '', password: '', is_admin: false }); const [error, setError] = useState('')
  return <div className="modal-backdrop" onMouseDown={e => e.target === e.currentTarget && close()}><form className="modal" onSubmit={async e => { e.preventDefault(); try { await api.createUser(value); done() } catch (x) { setError(message(x)) } }}><h2>新建本地用户</h2><div className="form-grid"><label>用户名<input required value={value.username} onChange={e => setValue({ ...value, username: e.target.value })} /></label><label>显示名称<input value={value.display_name} onChange={e => setValue({ ...value, display_name: e.target.value })} /></label><label className="wide">邮箱<input type="email" value={value.email} onChange={e => setValue({ ...value, email: e.target.value })} /></label><label className="wide">初始密码<input required type="password" minLength={12} value={value.password} onChange={e => setValue({ ...value, password: e.target.value })} /></label><label className="checkbox wide"><input type="checkbox" checked={value.is_admin} onChange={e => setValue({ ...value, is_admin: e.target.checked })} />管理员</label></div>{error && <p className="form-error">{error}</p>}<div className="modal-actions"><button type="button" className="quiet" onClick={close}>取消</button><button className="primary"><Save />创建</button></div></form></div>
}

function PoliciesView({ show }: { show: (s: string) => void }) {
  const [items, setItems] = useState<Policy[]>([]); const [editing, setEditing] = useState<Policy | null | undefined>()
  const load = () => api.policies().then(x => setItems(x.items)); useEffect(() => { void load() }, [])
  return <Page title="访问策略" action={<button className="primary" onClick={() => setEditing(null)}><ShieldCheck />新建策略</button>}><div className="policy-grid">{items.map(p => <article className="policy-card" key={p.id}><div><span className={`status ${p.enabled ? '' : 'off'}`}>{p.enabled ? '启用' : '停用'}</span><h2>{p.name}</h2><p>{p.description || '无描述'}</p></div><dl><div><dt>模式</dt><dd>{policyLabels[p.mode]}</dd></div><div><dt>规则</dt><dd>{p.rules.length}</dd></div></dl><div className="card-actions"><button className="quiet" onClick={() => setEditing(p)}>编辑</button><button className="icon-button danger" title="删除" onClick={async () => { if (confirm(`删除策略“${p.name}”？`)) { await api.deletePolicy(p.id); await load(); show('策略已删除') } }}><Trash2 /></button></div></article>)}</div>{editing !== undefined && <PolicyDialog policy={editing} close={() => setEditing(undefined)} done={async () => { setEditing(undefined); await load(); show('策略已保存') }} />}</Page>
}

function PolicyDialog({ policy, close, done }: { policy: Policy | null; close: () => void; done: () => void }) {
  const [value, setValue] = useState<PolicyInput>(policy ? { name: policy.name, description: policy.description, mode: policy.mode, enabled: policy.enabled, rules: policy.rules.map(({ kind, value }) => ({ kind, value })) } : blankPolicy); const [error, setError] = useState('')
  const setRules = (text: string) => setValue({ ...value, rules: text.split('\n').map(x => x.trim()).filter(Boolean).map(x => {
    if (x.startsWith('http://') || x.startsWith('https://')) return { kind: 'url_prefix' as const, value: x }
    if (x.startsWith('*.')) return { kind: 'domain_suffix' as const, value: x.slice(2) }
    if (x.includes('/')) return { kind: 'cidr' as const, value: x }
    return { kind: 'exact_host' as const, value: x }
  }) })
  const rulesText = value.rules.map(rule => rule.kind === 'domain_suffix' ? `*.${rule.value}` : rule.value).join('\n')
  return <div className="modal-backdrop" onMouseDown={e => e.target === e.currentTarget && close()}><form className="modal policy-modal" onSubmit={async e => { e.preventDefault(); try { if (policy) await api.updatePolicy(policy.id, value); else await api.createPolicy(value); done() } catch (x) { setError(message(x)) } }}><h2>{policy ? '编辑策略' : '新建策略'}</h2><div className="form-grid"><label>名称<input required value={value.name} onChange={e => setValue({ ...value, name: e.target.value })} /></label><label>模式<select value={value.mode} onChange={e => setValue({ ...value, mode: e.target.value as PolicyMode })}>{Object.entries(policyLabels).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</select></label><label className="wide">描述<input value={value.description} onChange={e => setValue({ ...value, description: e.target.value })} /></label><label className="wide">域名、URL 前缀或 CIDR 规则<textarea rows={7} placeholder={'example.com\n*.example.org\nhttps://example.net/docs/\n10.20.0.0/16'} value={rulesText} onChange={e => setRules(e.target.value)} /></label><label className="checkbox wide"><input type="checkbox" checked={value.enabled} onChange={e => setValue({ ...value, enabled: e.target.checked })} />启用策略</label></div>{error && <p className="form-error">{error}</p>}<div className="modal-actions"><button type="button" className="quiet" onClick={close}>取消</button><button className="primary"><Save />保存</button></div></form></div>
}

function LDAPView({ show }: { show: (s: string) => void }) {
  const [settings, setSettings] = useState<LDAPSettings>(); const [groups, setGroups] = useState<LDAPGroup[]>([]); const [policies, setPolicies] = useState<Policy[]>([]); const [error, setError] = useState('')
  const load = () => Promise.all([api.ldap(), api.groups(), api.policies()]).then(([l, g, p]) => { setSettings(l.settings); setGroups(g.items); setPolicies(p.items) }); useEffect(() => { void load() }, [])
  if (!settings) return <Page title="LDAP"><p>正在加载...</p></Page>
  const field = (key: keyof LDAPSettings, label: string) => <label>{label}<input value={String(settings[key] ?? '')} onChange={e => setSettings({ ...settings, [key]: e.target.value })} /></label>
  return <Page title="LDAP"><form className="settings-form" onSubmit={async e => { e.preventDefault(); setError(''); try { await api.updateLDAP(settings); show('LDAP 配置已保存') } catch (x) { setError(message(x)) } }}><div className="section-heading"><div><h2>目录连接</h2><p>密码和 CA 证书仅从容器 Secret 文件读取</p></div><label className="toggle"><input type="checkbox" checked={settings.enabled} onChange={e => setSettings({ ...settings, enabled: e.target.checked })} /><span /></label></div><div className="form-grid"><label>认证模式<select value={settings.mode} onChange={e => setSettings({ ...settings, mode: e.target.value })}><option value="search_bind">搜索绑定</option><option value="dn_template">DN 模板</option></select></label><label className="checkbox"><input type="checkbox" checked={settings.start_tls} onChange={e => setSettings({ ...settings, start_tls: e.target.checked })} />使用 StartTLS</label>{field('url', 'LDAP URL')}{field('base_dn', '用户 Base DN')}{field('bind_dn', '服务绑定 DN')}{field('user_filter', '用户过滤器')}{field('user_dn_template', '用户 DN 模板')}{field('username_attribute', '用户名属性')}{field('display_name_attribute', '显示名称属性')}{field('email_attribute', '邮箱属性')}{field('group_base_dn', '组 Base DN')}{field('group_filter', '组过滤器')}{field('group_name_attribute', '组名称属性')}</div>{error && <p className="form-error">{error}</p>}<div className="form-actions"><button type="button" className="quiet" onClick={async () => { try { await api.testLDAP(); show('LDAP 连接成功') } catch (x) { setError(message(x)) } }}><Wifi />测试连接</button><button className="primary"><Save />保存配置</button></div></form><section className="subsection"><h2>已同步组</h2><div className="table-wrap"><table><thead><tr><th>组</th><th>DN</th><th>最后同步</th><th>策略</th></tr></thead><tbody>{groups.map(g => <tr key={g.id}><td><strong>{g.name}</strong></td><td className="mono">{g.external_dn}</td><td>{formatDate(g.last_seen_at)}</td><td><PolicyPicker values={g.policy_ids} policies={policies} onChange={async ids => { await api.setGroupPolicies(g.id, ids); await load(); show('组策略已更新') }} /></td></tr>)}</tbody></table></div></section></Page>
}

function AuditView() { const [items, setItems] = useState<AuditEvent[]>([]); useEffect(() => { api.audit().then(x => setItems(x.items)) }, []); return <Page title="审计日志"><div className="table-wrap"><table><thead><tr><th>时间</th><th>事件</th><th>结果</th><th>目标</th><th>操作者</th></tr></thead><tbody>{items.map(x => <tr key={x.id}><td>{formatDate(x.created_at)}</td><td className="mono">{x.event_type}</td><td><span className={`status ${x.outcome === 'success' || x.outcome === 'allowed' ? '' : 'warn'}`}>{x.outcome}</span></td><td className="audit-target">{x.target}</td><td className="mono">{x.actor_user_id?.slice(0, 8) || 'system'}</td></tr>)}</tbody></table></div></Page> }

function PolicyPicker({ values, policies, onChange }: { values: string[]; policies: Policy[]; onChange: (ids: string[]) => void }) { return <details className="policy-picker"><summary>{values.length ? `${values.length} 项策略` : '未分配'}<ChevronDown /></summary><div>{policies.map(p => <label key={p.id}><input type="checkbox" checked={values.includes(p.id)} onChange={e => onChange(e.target.checked ? [...values, p.id] : values.filter(id => id !== p.id))} />{p.name}</label>)}</div></details> }
function ResourcePicker({ label, values, options, onChange }: { label: string; values: string[]; options: { id: string; name: string }[]; onChange: (ids: string[]) => void }) { return <details className="policy-picker"><summary>{values.length ? `${values.length} 项` : `无${label}`}<ChevronDown /></summary><div>{options.length ? options.map(option => <label key={option.id}><input type="checkbox" checked={values.includes(option.id)} onChange={e => onChange(e.target.checked ? [...values, option.id] : values.filter(id => id !== option.id))} />{option.name}</label>) : <span className="picker-empty">暂无可选项</span>}</div></details> }
function Page({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) { return <section><div className="page-heading"><h1>{title}</h1>{action}</div>{children}</section> }
function NavButton({ active, icon, label, onClick }: { active: boolean; icon: ReactNode; label: string; onClick: () => void }) { return <button className={active ? 'active' : ''} onClick={onClick}>{icon}<span>{label}</span></button> }
function message(error: unknown) { return error instanceof APIError || error instanceof Error ? error.message : '操作失败' }
function formatDate(value: string) { return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value)) }
