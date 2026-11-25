import { useState, type FormEvent } from 'react'
import { BookOpen } from 'lucide-react'
import { api } from '../lib/api'

export function Login({ onSuccess, initialError }: { onSuccess: () => void; initialError: string }) {
 const [token, setToken] = useState(''); const [error, setError] = useState(initialError); const [busy, setBusy] = useState(false)
 const submit = async (e: FormEvent) => { e.preventDefault(); setBusy(true); setError(''); try { await api('/api/login', { method: 'POST', body: JSON.stringify({ token: token.trim() }) }); onSuccess() } catch (e) { setError((e as Error).message) } finally { setBusy(false) } }
 return <div className="login-page"><div className="login-decoration"><BookOpen size={240} strokeWidth={0.7} /><span>READ. THINK. KEEP.</span></div><form className="login-card" onSubmit={submit}><span className="brand-mark"><BookOpen size={27} /></span><div className="eyebrow">READERHUB</div><h1>给阅读留一点空间。</h1><p>打开你的内网阅读空间，<br />继续那些值得慢慢看的内容。</p><label htmlFor="token">Hub 访问口令</label><input id="token" type="password" autoComplete="current-password" value={token} onChange={e => setToken(e.target.value)} required placeholder="输入管理员提供的访问口令" />{error && <div className="form-error" role="alert">{error}</div>}<button className="primary-button" disabled={busy || !token.trim()}>{busy ? '正在打开…' : '进入阅读空间'}</button><small>文章和想法，保存在你的内网 Hub。</small></form></div>
}
