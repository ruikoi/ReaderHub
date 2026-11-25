import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import type { Detail } from '../../lib/api'
export function renderArticle(article: Detail) {
 const md = new MarkdownIt({ html: true, linkify: true, typographer: false })
 const outline: { id: string; title: string; level: number }[] = []
 const tokens = md.parse(article.markdown, {})
 if (tokens[0]?.type === 'heading_open' && tokens[0].tag === 'h1' && tokens[1]?.content.trim() === article.title.trim()) tokens.splice(0, 3)
 tokens.forEach((token, index) => { if (token.type === 'heading_open') { const id = `rh-heading-${outline.length}`; token.attrSet('id', id); outline.push({ id, title: tokens[index + 1]?.content || '', level: Number(token.tag.slice(1)) }) } })
 const cleaned = DOMPurify.sanitize(md.renderer.render(tokens, md.options, {}), { FORBID_TAGS: ['style', 'script', 'iframe', 'form', 'object', 'embed', 'source', 'input', 'button'], FORBID_ATTR: ['style', 'srcset'] })
 const container = document.createElement('div'); container.innerHTML = cleaned
 const normalize = (s: string) => { try { return decodeURI(s) } catch { return s } }
 const resources = new Map(article.assets.map(a => [normalize(a.original), a]))
 container.querySelectorAll('img').forEach(img => { const src = img.getAttribute('src') || ''; const resource = resources.get(normalize(src)); if (resource) { const fragment = resource.original.includes('#') ? resource.original.slice(resource.original.indexOf('#')) : ''; img.src = `/api/items/${article.id}/assets/${resource.id}${fragment}`; img.loading = 'lazy'; img.decoding = 'async' } else { const missing = document.createElement('span'); missing.className = 'missing-image'; missing.textContent = `图片未随文档提交：${img.alt || '未命名图片'}`; img.replaceWith(missing) } })
 container.querySelectorAll('a').forEach(link => { const href = link.getAttribute('href') || ''; if (href.startsWith('#')) return; if (/^https?:\/\//i.test(href)) { link.target = '_blank'; link.rel = 'noopener noreferrer' } else { link.removeAttribute('href'); link.title = '此链接指向来源文件，未随本文提交'; link.classList.add('source-link') } })
 return { html: container.innerHTML, outline }
}
