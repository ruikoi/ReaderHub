export interface Asset { id: string; original: string; sha256: string; mime: string; size: number }
export interface Source { device_id: string; machine: string; project: string; path: string }
export interface Article { id: string; title: string; original_title: string; summary: string; tags: string[]; source: Source; assets: Asset[]; reading_state: 'to_read' | 'read'; saved: boolean; queued: boolean; progress: number; created_at: string; updated_at: string; note_count: number; body_sha256: string }
export interface Note { id: string; article_id: string; quote: string; text: string; created_at: string }
export interface Detail extends Article { markdown: string; notes: Note[] }
export class APIError extends Error { constructor(public status: number, message: string) { super(message) } }
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
 const response = await fetch(path, { credentials: 'same-origin', ...options, headers: { 'Content-Type': 'application/json', ...options.headers } })
 let body: unknown
 try { body = await response.json() } catch (error) { if (error instanceof DOMException && error.name === 'AbortError') throw error; throw new APIError(response.status, 'Hub 返回的数据格式无效') }
 if (!response.ok) throw new APIError(response.status, (body as { error?: string }).error || '请求失败，请稍后重试')
 return body as T
}
export const updateArticle = (id: string, patch: Partial<Article>) => api<Article>(`/api/items/${id}`, { method: 'PATCH', body: JSON.stringify(patch) })
export const dateLabel = (value: string) => new Date(value).toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' })
