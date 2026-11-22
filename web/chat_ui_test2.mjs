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

try {
  await page.goto(BASE + '/#/chat', { waitUntil: 'networkidle', timeout: 30000 })
  await page.waitForTimeout(1500)
  const hasInput = await page.locator('.chat-input textarea').count()
  log(`渲染: 输入框=${hasInput > 0}`)

  await page.fill('.chat-input textarea', MSG)
  await page.keyboard.press('Enter')
  log('已发送: ' + MSG)
  await page.waitForSelector('.msg-row.user .bubble', { timeout: 8000 })
  log('用户气泡 ✓')

  // 等助手非 pending 气泡，再多等 3s 让流式填完
  await page.waitForSelector('.msg-row.assistant .bubble:not(.pending)', { timeout: 90000 })
  await page.waitForTimeout(3000)
  const bubbles = await page.locator('.msg-row.assistant .bubble:not(.pending)').allTextContents()
  const content = bubbles.length ? bubbles[bubbles.length - 1] : ''
  log('助手回复(' + content.length + '字): ' + content.slice(0, 300).replace(/\n/g, ' '))

  const steps = await page.locator('.msg-steps').count()
  log('工具步骤条数: ' + steps)
  const sessions = await page.locator('.session-item').count()
  log('会话列表: ' + sessions)
  await page.screenshot({ path: 'chat_ui_result2.png' })
  log('截图 chat_ui_result2.png')
} catch (e) {
  log('异常: ' + e.message)
  await page.screenshot({ path: 'chat_ui_error2.png' }).catch(() => {})
} finally {
  log('控制台错误: ' + consoleErrors.length)
  consoleErrors.slice(0, 8).forEach((x) => console.log('  [ERR] ' + x.slice(0, 250)))
  await browser.close()
}
