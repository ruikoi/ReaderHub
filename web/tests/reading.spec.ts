import { test, expect } from '@playwright/test'
import { readFileSync, mkdirSync } from 'node:fs'
import { resolve } from 'node:path'
const token = () => readFileSync(process.env.READERHUB_TEST_TOKEN_FILE!, 'utf8').trim()

test.beforeEach(async ({ page }) => {
 page.on('pageerror', error => console.error('Browser runtime error:', error.stack))
 await page.goto('/')
 await page.getByLabel('Hub 访问口令').fill(token())
 await page.getByRole('button', { name: '进入阅读空间' }).click()
 await expect(page.getByRole('heading', { name: '留给下一次的阅读' })).toBeVisible()
})

test('read images, save, record thoughts and explicitly manage reading status', async ({ page }, info) => {
 const card = page.locator('.article-card').filter({ hasText: '给阅读留一点空间' }).first()
 await expect(card).toBeVisible()
 await card.locator('.article-title').click()
 await expect(page.locator('.article-header h1')).toHaveText('给阅读留一点空间')
 const image = page.locator('.prose img').first()
 await expect(image).toBeVisible()
 await expect.poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0)
 await expect(page.getByLabel('改变阅读状态')).toHaveValue('to_read')
 const save = page.getByRole('button', { name: '保存到文档库', exact: true })
 if (await save.isVisible()) await save.click()
 await expect(page.getByRole('button', { name: '已保存', exact: true })).toBeVisible()
 await expect(page.getByLabel('改变阅读状态')).toHaveValue('to_read')
 await page.getByRole('button', { name: /想法编辑/ }).click()
 await page.locator('.prose p').first().evaluate(node => { const range = document.createRange(); range.selectNodeContents(node); const selection = window.getSelection()!; selection.removeAllRanges(); selection.addRange(range) })
 await page.locator('.prose').dispatchEvent('mouseup')
 await expect(page.locator('.note-composer blockquote')).toBeVisible()
 await page.getByLabel('输入想法或问题').fill(`保留出处与图片，方便之后再读。${info.project.name}`)
 await page.getByRole('button', { name: '保存想法', exact: true }).click()
 await expect(page.locator('.note-card').filter({ hasText: info.project.name })).toBeVisible()
 const savedNote = page.locator('.note-card').filter({ hasText: info.project.name })
 await savedNote.getByLabel('编辑想法').click()
 await page.getByLabel('输入想法或问题').fill(`保留出处与图片，方便之后再读。${info.project.name}（已补充）`)
 await page.getByRole('button', { name: '保存修改', exact: true }).click()
 await expect(savedNote).toContainText('已补充')
 await savedNote.getByTitle('回到原文片段').click()
 expect(page.url()).toContain('#/article/')
 await expect(page.getByRole('button', { name: '研究整理尚未启用', exact: true })).toBeDisabled()
 await expect(page.getByLabel('改变阅读状态')).toHaveValue('to_read')
 await page.getByLabel('改变阅读状态').selectOption('read')
 await expect(page.getByLabel('改变阅读状态')).toHaveValue('read')
 await page.reload()
 await expect(page.getByLabel('改变阅读状态')).toHaveValue('read')
 await page.getByRole('button', { name: /想法编辑/ }).click()
 await expect(page.locator('.note-card').filter({ hasText: info.project.name })).toBeVisible()
 await page.getByLabel('改变阅读状态').selectOption('to_read')
 await expect(page.getByLabel('改变阅读状态')).toHaveValue('to_read')
 await page.getByRole('button', { name: '阅读模式', exact: true }).click()
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBeTruthy()
 mkdirSync(resolve('../.local/screenshots'), { recursive: true })
 await page.screenshot({ path: resolve(`../.local/screenshots/${info.project.name}-reader.png`), fullPage: true })
 await page.getByRole('button', { name: '返回列表', exact: true }).click()
 await expect(page.getByRole('heading', { name: '留给下一次的阅读' })).toBeVisible()
 await page.getByLabel('搜索文章').fill('保留出处与图片')
 await expect(page.locator('.article-card')).toHaveCount(1)
 await page.screenshot({ path: resolve(`../.local/screenshots/${info.project.name}-list.png`), fullPage: true })
})

test('untrusted Markdown cannot execute script or fetch an omitted image', async ({ page, request }, info) => {
 const manifest = { title: `安全展示验证-${info.project.name}`, markdown: '# 安全展示验证\n\n<script>window.readerhubInjected=true</script>\n\n<img src="https://invalid.example/track.png" onerror="window.readerhubInjected=true">', source: { device_id: 'browser-test', machine: '测试来源', project: '隔离测试', path: `/tests/${info.project.name}.md` }, assets: [] }
 const response = await request.post('/api/submissions', { headers: { Authorization: `Bearer ${token()}` }, multipart: { manifest: JSON.stringify(manifest) } })
 expect(response.status()).toBeLessThan(300)
 const receipt = await response.json()
 await page.goto(`/#/article/${receipt.id}`)
 await expect(page.locator('.prose')).toBeVisible()
 await expect(page.locator('.prose script')).toHaveCount(0)
 await expect(page.locator('.missing-image')).toBeVisible()
 expect(await page.evaluate(() => (window as unknown as Record<string, unknown>).readerhubInjected)).toBeUndefined()
 const removed = await request.delete(`/api/items/${receipt.id}?scope=queue`, { headers: { Authorization: `Bearer ${token()}` } })
 expect(removed.status()).toBe(200)
})


test('a cancelled response body does not replace the article list with invalid data', async ({ page }) => {
 await page.evaluate(() => {
  const original = window.fetch.bind(window)
  let pending = true
  window.fetch = async (input, init) => {
   if (pending && String(input).startsWith('/api/items?')) {
    pending = false
    const response = new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } })
    response.json = async () => { (window as unknown as Record<string, unknown>).bodyAbortSimulated = true; throw new DOMException('Response body cancelled', 'AbortError') }
    return response
   }
   return original(input, init)
  }
 })
 if (await page.getByLabel('打开导航').isVisible()) await page.getByLabel('打开导航').click()
 await page.getByRole('button', { name: '文档库', exact: true }).click()
 await page.waitForFunction(() => (window as unknown as Record<string, unknown>).bodyAbortSimulated === true)
 await expect(page.getByRole('heading', { name: '值得留下的内容' })).toBeVisible()
 await expect(page.locator('.error-banner')).toHaveCount(0)
 if (await page.getByLabel('打开导航').isVisible()) await page.getByLabel('打开导航').click()
 await page.getByRole('button', { name: '待读文章', exact: true }).click()
 await expect(page.locator('.article-card').first()).toBeVisible()
})
