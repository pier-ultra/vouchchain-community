import { chromium } from 'playwright'
const BASE = 'http://localhost:8000'
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1500, height: 950 } })
const errs = []
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()) })
page.on('pageerror', (e) => errs.push('PAGEERROR: ' + e.message))
await page.addInitScript(() => {
  localStorage.setItem('ch_token','dev-token'); localStorage.setItem('ch_role','admin')
  localStorage.setItem('ch_user','dev'); localStorage.setItem('ch_entity','0')
})
const log = (s) => console.log('[MCP-UI] ' + s)
let pass = 0, fail = 0
const ok = (c, s) => { if (c) { pass++; log('✅ ' + s) } else { fail++; log('❌ ' + s) } }

await page.goto(BASE + '/#/mcp-servers', { waitUntil: 'networkidle', timeout: 30000 })
await page.waitForTimeout(2500)
ok(page.url().includes('/mcp-servers'), '进入 /mcp-servers 页面')

// 页面标题
const title = await page.locator('.card-title').first().innerText().catch(() => '')
ok(title.includes('MCP 客户端'), '页面标题渲染: ' + title)

// 新增连接
await page.getByRole('button', { name: /新增连接/ }).click()
await page.waitForTimeout(800)
const dlg = page.locator('.el-dialog')
ok(await dlg.count() > 0, '新增对话框弹出')
await page.locator('.el-dialog input[placeholder*="cursor-server"]').first().fill('E2E 测试连接')
await page.locator('.el-dialog input[placeholder*="Streamable HTTP"]').first().fill('http://localhost:8000/mcp')
await page.locator('.el-dialog input[placeholder*="服务端加密存储"]').first().fill('testmcp-token-2026')
await page.getByRole('button', { name: '保存' }).click()
await page.waitForTimeout(2000)
ok((await page.locator('.el-table__row').count()) > 0, '新增后列表有数据')

// 工具预览
await page.getByRole('button', { name: '工具' }).first().click()
await page.waitForTimeout(1500)
const toolsDlg = page.locator('.el-dialog', { hasText: '工具列表' })
ok(await toolsDlg.count() > 0, '工具对话框弹出')
const toolRows = await toolsDlg.locator('.el-table__row').count()
ok(toolRows === 9, '工具列表 9 个（实际 ' + toolRows + '）')
await page.getByRole('button', { name: '关闭', exact: true }).click()
await page.waitForTimeout(600)

// 测试连接（行内）
await page.getByRole('button', { name: '测试' }).first().click()
await page.waitForTimeout(2000)
const testRes = page.locator('.el-dialog', { hasText: '连接成功' })
ok(await testRes.count() > 0, '行内测试连接成功')
await page.getByRole('button', { name: '知道了' }).click()
await page.waitForTimeout(500)

await page.screenshot({ path: 'shots/mcp_servers.png' })
log(`========== 通过 ${pass} / 失败 ${fail} ==========`)
log('控制台错误: ' + errs.length)
errs.slice(0, 5).forEach((e) => console.log('  [ERR] ' + e.slice(0, 200)))
await browser.close()
