import { chromium } from 'playwright'

const BASE = 'http://localhost:8000'
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1500, height: 950 } })
const errs = []
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()) })
page.on('pageerror', (e) => errs.push('PAGEERROR: ' + e.message))
await page.addInitScript(() => {
  localStorage.setItem('ch_token', 'dev-token')
  localStorage.setItem('ch_role', 'viewer')
  localStorage.setItem('ch_user', 'verify')
  localStorage.setItem('ch_entity', '0')
})
const log = (s) => console.log('[VERIFY] ' + s)
let pass = 0, fail = 0
const ok = (c, s) => { if (c) { pass++; log('✅ ' + s) } else { fail++; log('❌ ' + s) } }

// ===== 1. 侧边栏「关于我们」菜单 =====
await page.goto(BASE + '/#/dashboard', { waitUntil: 'networkidle', timeout: 30000 })
await page.waitForTimeout(2500)
const aboutMenu = page.locator('.el-menu', { hasText: '关于我们' })
ok(await aboutMenu.count() > 0, '侧边栏出现「关于我们」菜单')
// 点击进入 about
if (await aboutMenu.count() > 0) {
  await page.locator('.el-sub-menu__title', { hasText: '关于我们' }).first().click().catch(() => {})
  await page.waitForTimeout(600)
  await page.locator('.el-menu-item', { hasText: '关于我们' }).first().click().catch(() => {})
  await page.waitForTimeout(2500)
  ok(page.url().includes('/about'), '点击后进入 /about')
  const aboutTxt = await page.evaluate(() => document.body.innerText || '')
  ok(aboutTxt.includes('VouchChain') && aboutTxt.includes('合规'), '关于我们页面渲染内容')
}

// ===== 2. 关于页二维码点击 → 弹窗 =====
const aboutQr = page.locator('.qr-wrapper').first()
if (await aboutQr.count() > 0) {
  await aboutQr.click()
  await page.waitForTimeout(1000)
  const dialog = page.locator('.qr-modal, .el-dialog', { hasText: '扫码加入' }).first()
  ok(await dialog.count() > 0, '关于页二维码点击弹出对话框')
  const img = dialog.locator('img.qr-image')
  const imgBox = await img.boundingBox().catch(() => null)
  ok(imgBox && imgBox.width > 300, '弹窗内二维码大图（宽>300px: ' + Math.round(imgBox?.width || 0) + 'px）')
  await page.screenshot({ path: 'verify_about_qr.png' })
  // 关闭（X）
  await page.locator('.qr-modal .el-dialog__headerbtn, .qr-modal .el-dialog__header .el-dialog__close').first().click().catch(() => {})
  await page.waitForTimeout(800)
  const dialogGone = await page.locator('.qr-modal').count() === 0 || await page.locator('.el-dialog').first().isHidden().catch(() => true)
  ok(dialogGone, '弹窗可关闭')
} else {
  ok(false, '关于页未找到二维码')
}

// ===== 3. 登录页二维码点击 → 弹窗 =====
await page.goto(BASE + '/#/login', { waitUntil: 'networkidle', timeout: 30000 })
await page.waitForTimeout(2000)
const loginQr = page.locator('.community-qr').first()
ok(await loginQr.count() > 0, '登录页存在二维码')
if (await loginQr.count() > 0) {
  await loginQr.click()
  await page.waitForTimeout(1000)
  const dlg = page.locator('.qr-modal, .el-dialog', { hasText: '扫码加入' }).first()
  ok(await dlg.count() > 0, '登录页二维码点击弹出对话框')
  const img2 = dlg.locator('img.qr-image')
  const box2 = await img2.boundingBox().catch(() => null)
  ok(box2 && box2.width > 300, '弹窗大图（宽>300px）')
  await page.screenshot({ path: 'verify_login_qr.png' })
  // 点击遮罩关闭
  await page.mouse.click(20, 20)
  await page.waitForTimeout(800)
}

log(`========== 通过 ${pass} / 失败 ${fail} ==========`)
log('控制台错误: ' + errs.length)
errs.slice(0, 5).forEach((e) => console.log('  [ERR] ' + e.slice(0, 200)))
await browser.close()
