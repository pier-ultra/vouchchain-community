import { chromium } from 'playwright'

const BASE = 'http://localhost:8000'
const MSG = '你好，帮我看看系统里有多少份合同？'

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1400, height: 900 } })

const consoleErrors = []
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
page.on('pageerror', (e) => consoleErrors.push('PAGEERROR: ' + e.message))

// devMode 下后端旁路鉴权；前端路由守卫需要 token/角色，预置 localStorage
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

  const hasTitle = await page.locator('text=AI 助手').count()
  const hasInput = await page.locator('.chat-input textarea').count()
  const hasWelcome = await page.locator('.chat-welcome').count()
  log(`渲染检查 → 标题:${hasTitle>0} 输入框:${hasInput>0} 欢迎区:${hasWelcome>0}`)
  await page.screenshot({ path: 'chat_ui_render.png' })

  if (hasInput > 0) {
    await page.fill('.chat-input textarea', MSG)
    await page.keyboard.press('Enter')
    log('已发送: ' + MSG)

    await page.waitForSelector('.msg-row.user .bubble', { timeout: 8000 })
    log('用户气泡出现 ✓')

    let replied = false, content = ''
    try {
      await page.waitForSelector('.msg-row.assistant .bubble:not(.pending)', { timeout: 60000 })
      const bubbles = await page.locator('.msg-row.assistant .bubble:not(.pending)').allTextContents()
      content = bubbles.length ? bubbles[bubbles.length - 1] : ''
      replied = true
    } catch { replied = false }
    log(replied ? '助手回复出现 ✓' : '助手回复超时（60s）')
    if (content) log('助手回复(前200字): ' + content.slice(0, 200).replace(/\n/g, ' '))

    const sessions = await page.locator('.session-item').count()
    log(`会话列表条数: ${sessions}`)
    await page.screenshot({ path: 'chat_ui_result.png' })
    log('截图 chat_ui_result.png')
  }
} catch (e) {
  log('异常: ' + e.message)
  await page.screenshot({ path: 'chat_ui_error.png' }).catch(() => {})
} finally {
  log('控制台错误数: ' + consoleErrors.length)
  consoleErrors.slice(0, 8).forEach((x) => console.log('  [ERR] ' + x.slice(0, 250)))
  await browser.close()
}
