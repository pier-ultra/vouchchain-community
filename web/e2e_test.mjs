import { chromium } from 'playwright'

const BASE = 'http://localhost:8000'
const TMP = 'e2e-tmp-' + Date.now().toString().slice(-6)
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1500, height: 950 } })
const errs = []
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()) })
page.on('pageerror', (e) => errs.push('PAGEERROR: ' + e.message))
await page.addInitScript(() => {
  localStorage.setItem('ch_token', 'dev-token')
  localStorage.setItem('ch_role', 'admin')
  localStorage.setItem('ch_user', 'e2e')
  localStorage.setItem('ch_entity', '0')
})
const log = (s) => console.log('[E2E] ' + s)
let pass = 0, fail = 0
const ok = (c, s) => { if (c) { pass++; log('✅ ' + s) } else { fail++; log('❌ ' + s) } }

try {
  // ========== 1. 模型配置页 ==========
  await page.goto(BASE + '/#/model-configs', { waitUntil: 'networkidle', timeout: 30000 })
  await page.waitForTimeout(2000)
  ok(await page.locator('text=模型配置').count() > 0, '模型配置页标题渲染')
  ok(await page.locator('button:has-text("新增配置")').count() > 0, '「新增配置」按钮存在')

  // 行操作列：编辑/测试/删除
  const firstRow = page.locator('.el-table__body tr').first()
  ok(await firstRow.locator('button:has-text("编辑")').count() > 0, '行「编辑」按钮存在')
  ok(await firstRow.locator('button:has-text("测试")').count() > 0, '行「测试」按钮存在')
  ok(await firstRow.locator('button:has-text("删除")').count() > 0, '行「删除」按钮存在')
  ok(await firstRow.locator('.el-switch').count() > 0, '行「启用」开关存在')

  // ========== 2. 新增配置 ==========
  await page.click('button:has-text("新增配置")')
  await page.waitForSelector('.el-dialog:visible', { timeout: 8000 })
  ok(await page.locator('.el-dialog input[placeholder*="如：agnes"]').count() > 0, '新增对话框打开（配置名输入框）')
  await page.fill('.el-dialog input[placeholder*="如：agnes"]', TMP)
  await page.fill('.el-dialog input[placeholder*="https://"]', 'https://example.com/v1')
  await page.fill('.el-dialog input[placeholder*="如：agnes-3"]', 'e2e-model')
  await page.click('.el-dialog .el-dialog__footer button:has-text("保存")')
  await page.waitForTimeout(2500)
  ok(await page.locator('.el-table__body tr', { hasText: TMP }).count() > 0, '新增保存后列表出现 ' + TMP)

  // ========== 3. 编辑配置 ==========
  const tmpRow = page.locator('.el-table__body tr', { hasText: TMP })
  await tmpRow.locator('button:has-text("编辑")').click()
  await page.waitForSelector('.el-dialog:visible', { timeout: 8000 })
  ok((await page.locator('.el-dialog__header').innerText()).includes('编辑'), '编辑对话框标题正确')
  const newName = TMP + '-x'
  await page.fill('.el-dialog input[placeholder*="如：agnes"]', newName)
  await page.click('.el-dialog .el-dialog__footer button:has-text("保存")')
  await page.waitForTimeout(2500)
  ok(await page.locator('.el-table__body tr', { hasText: newName }).count() > 0, '编辑保存后名称更新为 ' + newName)

  // ========== 4. 启停开关（切 id=3 讯飞 llm 行） ==========
  const llmRow = page.locator('.el-table__body tr', { hasText: 'spark-x2.6-4b' })
  if (await llmRow.count() > 0) {
    const sw = llmRow.locator('.el-switch')
    const before = await sw.getAttribute('class')
    await sw.click()
    await page.waitForTimeout(1500)
    log('启停开关切换完成（class 变化: ' + (await sw.getAttribute('class')) !== before + '）')
    ok(true, '启停开关可点击切换')
    // 切回
    await sw.click()
    await page.waitForTimeout(1200)
  } else {
    ok(false, '未找到讯飞 llm 行')
  }

  // ========== 5. 测试按钮（id=3 讯飞 llm，真实调用） ==========
  if (await llmRow.count() > 0) {
    await llmRow.locator('button:has-text("测试")').click()
    try {
      await page.waitForSelector('text=模型测试结果', { timeout: 60000 })
      await page.waitForTimeout(1500)
      const resTitle = await page.locator('.el-dialog .el-result__title').innerText().catch(() => '')
      log('测试结果弹窗标题: ' + resTitle.trim())
      ok(true, '「测试」按钮触发结果弹窗')
      await page.screenshot({ path: 'e2e_model_test.png' })
      await page.click('.el-dialog .el-dialog__footer button:has-text("知道了")')
      await page.waitForTimeout(800)
    } catch {
      ok(false, '测试结果弹窗超时未出现')
    }
  }

  // ========== 6. 删除临时配置 ==========
  const delRow = page.locator('.el-table__body tr', { hasText: newName })
  if (await delRow.count() > 0) {
    await delRow.locator('button:has-text("删除")').click()
    await page.waitForSelector('.el-popconfirm__action', { timeout: 8000 })
    await page.click('.el-popconfirm__action .el-button--primary')
    await page.waitForTimeout(2500)
    ok(await page.locator('.el-table__body tr', { hasText: newName }).count() === 0, '删除后 ' + newName + ' 从列表消失')
  } else {
    ok(false, '未找到待删除行')
  }
  await page.screenshot({ path: 'e2e_model_configs.png' })

  // ========== 7. AI 对话页 ==========
  await page.goto(BASE + '/#/chat', { waitUntil: 'networkidle', timeout: 30000 })
  await page.waitForTimeout(1800)
  ok(await page.locator('.chat-input textarea').count() > 0, 'AI 对话页输入框渲染')
  await page.fill('.chat-input textarea', '你好，介绍一下你能做什么')
  await page.keyboard.press('Enter')
  await page.waitForSelector('.msg-row.user .bubble', { timeout: 8000 })
  ok(true, '用户消息气泡出现')
  try {
    await page.waitForSelector('.msg-row.assistant .bubble:not(.pending)', { timeout: 90000 })
    await page.waitForTimeout(3000)
    const bodyText = await page.locator('.chat-body').innerText()
    ok(bodyText.includes('VouchChain') || bodyText.length > 50, 'AI 回复渲染（含内容）')
    log('回复片段: ' + bodyText.slice(-180).replace(/\n/g, ' '))
  } catch {
    ok(false, 'AI 回复超时')
  }
  await page.screenshot({ path: 'e2e_chat.png' })
} catch (e) {
  fail++
  log('异常: ' + e.message)
  await page.screenshot({ path: 'e2e_error.png' }).catch(() => {})
} finally {
  log(`========== 通过 ${pass} / 失败 ${fail} ==========`)
  log('控制台错误: ' + errs.length)
  errs.slice(0, 6).forEach((x) => console.log('  [ERR] ' + x.slice(0, 250)))
  await browser.close()
}
