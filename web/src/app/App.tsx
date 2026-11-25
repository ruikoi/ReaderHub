import { useCallback, useEffect, useState } from 'react'
import { BookOpen, Bookmark, Check, ChevronDown, Library, List, LogOut, Menu, Search, X } from 'lucide-react'
import { api, APIError, updateArticle, type Article } from '../lib/api'
import { Login } from '../components/Login'
import { MetadataDialog } from '../components/MetadataDialog'
import { ArticleCard } from '../features/queue/ArticleCard'
import { Reader } from '../features/reader/Reader'

type Scope = 'queue' | 'library'
function routeID() { return location.hash.match(/^#\/article\/([a-f0-9]{32})$/)?.[1] || '' }
export function App() {
 const [authenticated, setAuthenticated] = useState<boolean | null>(null)
 const [scope, setScope] = useState<Scope>('queue')
 const [articleID, setArticleID] = useState(routeID)
 const [items, setItems] = useState<Article[]>([])
 const [query, setQuery] = useState('')
 const [stateFilter, setStateFilter] = useState('all')
 const [device, setDevice] = useState('all')
 const [tag, setTag] = useState('all')
 const [loading, setLoading] = useState(true)
 const [error, setError] = useState('')
 const [notice, setNotice] = useState('')
 const [menuOpen, setMenuOpen] = useState(false)
 const [editing, setEditing] = useState<Article | null>(null)
 const [refresh, setRefresh] = useState(0)
 useEffect(() => { api('/api/session').then(() => setAuthenticated(true)).catch(e => { if (e instanceof APIError && e.status === 401) setAuthenticated(false); else { setError(e.message); setAuthenticated(false) } }) }, [])
 useEffect(() => { const change = () => setArticleID(routeID()); addEventListener('hashchange', change); return () => removeEventListener('hashchange', change) }, [])
 const flash = useCallback((text: string) => { setNotice(text); setTimeout(() => setNotice(''), 3500) }, [])
 useEffect(() => { if (!authenticated) return; setError(''); const controller = new AbortController(); setLoading(true); const timer = setTimeout(() => { api<Article[]>(`/api/items?scope=${scope}&q=${encodeURIComponent(query)}`, { signal: controller.signal }).then(setItems).catch(e => { if (e.name !== 'AbortError') setError(e.message) }).finally(() => { if (!controller.signal.aborted) setLoading(false) }) }, 180); return () => { clearTimeout(timer); controller.abort() } }, [authenticated, scope, query, refresh])
 const chooseScope = (value: Scope) => { setScope(value); setDevice('all'); setTag('all'); location.hash = ''; setArticleID(''); setMenuOpen(false) }
 const mutate = async (article: Article, patch: Partial<Article>) => { if (patch.saved === false && !article.queued) { if (!await remove(article)) throw new Error('cancelled'); return article } try { const updated = await updateArticle(article.id, patch); setItems(old => old.map(a => a.id === article.id ? { ...a, ...updated } : a)); setRefresh(n => n + 1); return updated } catch (e) { flash((e as Error).message); throw e } }
 const remove = async (article: Article) => { if (!confirm(scope === 'library' ? '移除此收藏？如果文章仍在待读列表，阅读材料会继续保留。' : !article.saved && article.note_count > 0 ? `移出待读将删除本文和 ${article.note_count} 条想法，是否继续？` : '移出待读？已经保存的收藏不受影响。')) return false; try { await api(`/api/items/${article.id}?scope=${scope}`, { method: 'DELETE' }); setRefresh(n => n + 1); if (articleID === article.id) location.hash = ''; flash('已移除'); return true } catch (e) { flash((e as Error).message); return false } }
 const visible = items.filter(a => (stateFilter === 'all' || a.reading_state === stateFilter) && (device === 'all' || a.source.device_id === device) && (tag === 'all' || a.tags.includes(tag)))
 const devices = [...new Map(items.map(a => [a.source.device_id, a.source.machine])).entries()]
 const tags = [...new Set(items.flatMap(a => a.tags))]
 if (authenticated === null) return <div className="startup"><BookOpen size={32} /><p>正在打开阅读空间…</p></div>
 if (!authenticated) return <Login initialError={error} onSuccess={() => { setAuthenticated(true); setError('') }} />
 return <div className={`shell ${articleID ? 'reading-shell' : ''}`}>
  {menuOpen && <div className="sidebar-backdrop" onClick={() => setMenuOpen(false)} />}
  <aside className={`sidebar ${menuOpen ? 'mobile-open' : ''}`}>
   <a className="brand" href="#" onClick={() => chooseScope('queue')}><span className="brand-mark"><BookOpen size={24} strokeWidth={1.8} /></span><span>ReaderHub<small>个人阅读空间</small></span></a>
   <div className="nav-caption">我的空间</div>
   <button className={`nav-item ${scope === 'queue' && !articleID ? 'active' : ''}`} onClick={() => chooseScope('queue')}><List size={19} />待读文章</button>
   <button className={`nav-item ${scope === 'library' && !articleID ? 'active' : ''}`} onClick={() => chooseScope('library')}><Library size={19} />文档库</button>
   <div className="sidebar-divider" /><div className="nav-caption">阅读，从容一点</div><p className="sidebar-description">把值得看的内容放在这里，<br />留一点时间给思考。</p>
   <div className="sidebar-bottom"><div className="local-badge"><span />内网阅读</div><button className="text-button logout" onClick={async () => { await api('/api/logout', { method: 'POST' }); setAuthenticated(false); setItems([]) }}><LogOut size={15} />退出空间</button></div>
  </aside>
  <main className="main">
   <header className="topbar"><div className="topbar-location"><button className="icon-button mobile-menu" aria-label="打开导航" onClick={() => setMenuOpen(true)}><Menu size={22} /></button><span>我的空间</span><span className="crumb-divider">/</span><strong>{articleID ? '文章阅读' : scope === 'queue' ? '待读文章' : '文档库'}</strong></div><div className="topbar-end"><span className="local-label">私人 · 内网</span><span className="avatar">阅</span></div></header>
   {articleID ? <Reader key={articleID} refresh={refresh} id={articleID} flash={flash} onBack={() => { location.hash = '' }} onChanged={() => setRefresh(n => n + 1)} onEdit={setEditing} /> : <section className="collection">
    <div className="page-heading"><div><div className="eyebrow">YOUR READING SPACE</div><h1>{scope === 'queue' ? '留给下一次的阅读' : '值得留下的内容'}</h1><p>{scope === 'queue' ? '文章已经在这里。选一篇，慢慢读。' : '你亲自选中的文章，随时回来再读。'}</p></div><div className="collection-count"><BookOpen size={18} /><strong>{items.length}</strong><span>篇文章</span></div></div>
    <div className="collection-toolbar"><label className="search-box"><Search size={19} /><input aria-label="搜索文章" value={query} onChange={e => setQuery(e.target.value)} placeholder="搜索标题、正文、标签或我的想法…" />{query && <button aria-label="清空搜索" onClick={() => setQuery('')}><X size={16} /></button>}</label><div className="filters"><label className="select-wrap"><select aria-label="阅读状态" value={stateFilter} onChange={e => setStateFilter(e.target.value)}><option value="all">全部状态</option><option value="to_read">待阅读</option><option value="read">已读</option></select><ChevronDown size={14} /></label><label className="select-wrap"><select aria-label="来源设备" value={device} onChange={e => setDevice(e.target.value)}><option value="all">全部来源</option>{devices.map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select><ChevronDown size={14} /></label>{tags.length > 0 && <label className="select-wrap"><select aria-label="主题标签" value={tag} onChange={e => setTag(e.target.value)}><option value="all">全部标签</option>{tags.map(t => <option key={t}>{t}</option>)}</select><ChevronDown size={14} /></label>}</div></div>
    <div className="section-label"><span>{query ? '搜索结果' : scope === 'queue' ? '全部待读' : '已保存文章'} <b>{visible.length}</b></span><span>按提交时间排列</span></div>
    {error && <div className="error-banner" role="alert">{error}<button onClick={() => { setError(''); setRefresh(n => n + 1) }}>重试</button></div>}
    {loading ? <div className="loading-state">正在加载文章…</div> : visible.length === 0 ? <div className="empty-state"><span className="empty-icon">{scope === 'queue' ? <BookOpen size={32} /> : <Bookmark size={32} />}</span><h2>{query || stateFilter !== 'all' || device !== 'all' || tag !== 'all' ? '没有找到匹配的文章' : scope === 'queue' ? '你的下一篇好文章，从这里开始' : '把值得重读的文章留下来'}</h2><p>{scope === 'queue' ? '在来源电脑上提交 Markdown，正文和图片会一起来到这里。' : '阅读时点击“保存到文档库”，以后就能在这里找到。'}</p>{scope === 'queue' && <code>readerhub-cli add ./article.md</code>}</div> : <div className="article-list">{visible.map(article => <ArticleCard key={article.id} article={article} onOpen={() => { location.hash = `/article/${article.id}`; window.scrollTo(0, 0) }} onSave={() => { mutate(article, { saved: !article.saved }).then(() => flash(article.saved ? '已取消收藏' : '已保存到文档库')).catch(() => {}) }} onEdit={() => setEditing(article)} onRemove={() => remove(article)} onTag={setTag} />)}</div>}
    <div className="collection-footer"><span>只留下你选择的内容</span><span>ReaderHub</span></div>
   </section>}
  </main>
  {notice && <div role="status" className="toast"><Check size={17} />{notice}</div>}
  {editing && <MetadataDialog article={editing} onClose={() => setEditing(null)} onSave={async (title, tags) => { await mutate(editing, { title, tags }); setEditing(null); flash('文章信息已更新') }} />}
 </div>
}
