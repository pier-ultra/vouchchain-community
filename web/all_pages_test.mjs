import { chromium } from 'playwright'
import fs from 'fs'

const BASE = 'http://localhost:8000'
const SHOTS = 'shots'
if (!fs.existsSync(SHOTS)) fs.mkdirSync(SHOTS, { recursive: true })

const ROUTES = [
  { path: '/login', name: '登录', title: 'Login' },
  { path: '/dashboard', name: '工作概览', title: 'Dashboard' },
  { path: '/contracts', name: '合同台账', title: 'Contracts' },
  { path: '/generate', name: 'AI 生成', title: 'Generate' },
  { path: '/extraction', name: '任务中心', title: 'Extraction' },
  { path: '/obligations', name: '义务看板', title: 'Obligations' },
  { path: '/contract-templates', name: '合同模板库', title: 'ContractTemplates' },
  { path: '/query', name: '智能问答', title: 'Query' },
  { path: '/sign', name: '签署管理', title: 'Sign' },
  { path: '/approvals', name: '审批中心', title: 'Approvals' },
  { path: '/invoices', name: '发票中心', title: 'InvoiceCenter' },
  { path: '/invoice-ledger', name: '发票台账', title: 'InvoiceLedger' },
  { path: '/accounting', name: '应付与做账', title: 'Accounting' },
  { path: '/model-configs', name: '模型配置', title: 'ModelConfigs' },
  { path: '/organization', name: '组织与主体', title: 'Organization' },
  { path: '/master-data', name: '主数据', title: 'MasterData' },
  { path: '/archive-schedules', name: '归档调度', title: 'ArchiveSchedule' },
  { path: '/long-term', name: '长期保存', title: 'LongTerm' },
  { path: '/compliance', name: '合规策略', title: 'Compliance' },
  { path: '/mdm-dedup', name: '主数据去重', title: 'MdmDedup' },
  { path: '/roles', name: '角色权限', title: 'Roles' },
  { path: '/chat', name: 'AI 助手', title: 'Chat' },
  { path: '/files', name: '文件中心', title: 'FileCenter' },
  { path: '/invoice-class', name: '发票分类', title: 'InvoiceClass' },
  { path: '/expense', name: '费控管理', title: 'ExpenseControl' },
  { path: '/skills', name: 'AI 技能', title: 'Skills' },
  { path: '/revrec', name: '收入确认', title: 'RevRec' },
  { path: '/seals', name: '用印管理', title: 'Seals' },
  { path: '/isolation', name: '强隔离策略', title: 'Isolation' },
  { path: '/identity-sync', name: '身份同步', title: 'IdentitySync' },
  { path: '/party-profiles', name: '相对方画像', title: 'PartyProfile' },
  { path: '/dengbao', name: '等保证据', title: 'Dengbao' },
  { path: '/duban', name: '公文督办', title: 'Duban' },
  { path: '/audit-log', name: '审计日志', title: 'AuditLog' },
  { path: '/api-docs', name: '接口文档', title: 'ApiDocs' },
  { path: '/about', name: '关于我们', title: 'AboutUs' },
  { path: '/mcp-servers', name: 'MCP 客户端', title: 'McpServers' },
]

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1500, height: 950 } })
await page.addInitScript(() => {
  localStorage.setItem('ch_token', 'dev-token')
  localStorage.setItem('ch_role', 'admin')
  localStorage.setItem('ch_user', 'audit')
  localStorage.setItem('ch_entity', '0')
})

const results = []
for (const r of ROUTES) {
  const errs = []
  const perPage = (m) => { if (m.type() === 'error') errs.push('[console] ' + m.text()) }
  const onPageErr = (e) => errs.push('[pageerror] ' + e.message)
  page.on('console', perPage)
  page.on('pageerror', onPageErr)

  let bodyLen = 0
  let navOK = false
  let err = ''
  try {
    await page.goto(BASE + '/#/' + r.path.replace(/^\//, ''), { waitUntil: 'networkidle', timeout: 30000 })
    await page.waitForTimeout(2500)
    const txt = await page.evaluate(() => document.body.innerText || '')
    bodyLen = txt.length
    navOK = bodyLen > 15
    // 页面是否有明显 Vue 错误白屏
    if (!navOK) err = '疑似白屏（body 文本过短）'
  } catch (e) {
    err = '加载异常: ' + e.message
  }
  await page.screenshot({ path: `${SHOTS}/${r.title}.png` }).catch(() => {})
  page.off('console', perPage)
  page.off('pageerror', onPageErr)

  results.push({ ...r, bodyLen, ok: navOK && !err, err, consoleErrs: errs.slice(0, 8) })
}

await browser.close()

// ===== 汇总 =====
const bad = results.filter((r) => !r.ok)
const withConsole = results.filter((r) => r.consoleErrs.length > 0)
console.log('========== 全页面巡检汇总 ==========')
console.log(`页面总数: ${results.length}  渲染异常: ${bad.length}  有控制台错误: ${withConsole.length}`)
console.log('\n--- 渲染异常页 ---')
if (!bad.length) console.log('（无）')
bad.forEach((r) => console.log(`  ❌ ${r.path} [${r.name}] ${r.err}`))
console.log('\n--- 有控制台错误的页面 ---')
if (!withConsole.length) console.log('（无）')
withConsole.forEach((r) => {
  console.log(`  ⚠️ ${r.path} [${r.name}] ${r.consoleErrs.length}条`)
  r.consoleErrs.slice(0, 3).forEach((e) => console.log('      ' + e.slice(0, 180)))
})
console.log('\n--- 全部页面渲染状态 ---')
results.forEach((r) => console.log(`  ${r.ok ? '✅' : '❌'} ${r.path.padEnd(20)} 文本${String(r.bodyLen).padStart(6)}  ${r.consoleErrs.length ? '⚠️' + r.consoleErrs.length + 'err' : ''}`))
console.log(`\n截图目录: ${SHOTS}/`)
