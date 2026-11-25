import { useState } from 'react'
import { X } from 'lucide-react'
import type { Article } from '../lib/api'

export function MetadataDialog({ article, onClose, onSave }: { article: Article; onClose: () => void; onSave: (title: string, tags: string[]) => Promise<void> }) {
 const [title, setTitle] = useState(article.title); const [tags, setTags] = useState(article.tags.join('，')); const [busy, setBusy] = useState(false); const [error, setError] = useState('')
 return <div className="modal-overlay" onClick={onClose}><form className="metadata-dialog" role="dialog" aria-modal="true" aria-label="编辑文章信息" onClick={e => e.stopPropagation()} onSubmit={async e => { e.preventDefault(); setBusy(true); try { await onSave(title, tags.split(/[,，]/).map(t => t.trim()).filter(Boolean)) } catch (e) { setError((e as Error).message); setBusy(false) } }}><div className="dialog-heading"><h2>编辑文章信息</h2><button className="icon-button" type="button" aria-label="关闭" onClick={onClose}><X size={20} /></button></div><p>只修改 Hub 中的展示信息，原文保持不变。</p><label>展示标题<input value={title} onChange={e => setTitle(e.target.value)} required maxLength={200} /></label><label>主题标签<input value={tags} onChange={e => setTags(e.target.value)} placeholder="例如：Linux，并发，内存屏障" /></label><small>多个标签用逗号分隔，不需要预先建立分类。</small>{error && <p className="form-error">{error}</p>}<div className="dialog-actions"><button type="button" className="secondary-button" onClick={onClose}>取消</button><button className="primary-button" disabled={busy || !title.trim()}>{busy ? '保存中…' : '保存修改'}</button></div></form></div>
}
