import { chromium } from 'playwright'

const BASE = 'http://localhost:8000'
const MSG = '你好'
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1400, height: 900 } })
const consoleErrors = []
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
page.on('pageerror', (e) => consoleErrors.push('PAGEERROR: ' + e.message))
await page.addInitScript(() => {
  localStorage.setItem('ch_token', 'dev-token')
  localStorage.setItem('ch_role', 'admin')
  localStorage.setItem('ch_user', 'tester')
  localStorage.setItem('ch_entity', '0')
})
const log = (s) => console.log('[UI]', s)

// 捕获 SSE 响应
let sseSnippet = ''
page.on('response', async (resp) => {
  if (resp.url().includes('/api/chat/stream')) {
    try {
      const text = await resp.text()
      sseSnippet = text.slice(0, 600)
    } catch {}
  }
})

try {
  await page.goto(BASE + '/#/chat', { waitUntil: 'networkidle', timeout: 30000 })
  await page.waitForTimeout(1500)
  await page.fill('.chat-input textarea', MSG)
  await page.keyboard.press('Enter')
  log('已发送: ' + MSG)
  await page.waitForSelector('.msg-row.assistant .bubble:not(.pending)', { timeout: 90000 })
  await page.waitForTimeout(3000)
  const bodyText = await page.locator('.chat-body').innerText()
  log('=== chat-body innerText ===')
  log(bodyText.slice(0, 500))
  log('=== SSE 响应前 400 字 ===')
  log(sseSnippet.slice(0, 400))
} catch (e) {
  log('异常: ' + e.message)
} finally {
  log('控制台错误: ' + consoleErrors.length)
  consoleErrors.slice(0, 8).forEach((x) => console.log('  [ERR] ' + x.slice(0, 250)))
  await browser.close()
}
