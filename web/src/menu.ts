// 侧栏菜单单一来源：大条目（分组）→ 小菜单（子项）。
// 18 个页面平铺在一级菜单里太挤，这里按业务域归成 5 组；
// 分组只做导航归类；可见菜单由后端 /api/me 下发，前端按路径过滤（空组自动隐藏）。

export interface MenuItem {
  path: string
  title: string
  icon: string // Element Plus 图标组件名（main.ts 已全量全局注册）
}

export interface MenuGroup {
  key: string // 分组 index，加 grp- 前缀避免与路由 path 冲突
  title: string
  icon: string
  items: MenuItem[]
}

export const menuGroups: MenuGroup[] = [
  {
    key: 'grp-common', title: '常用功能', icon: 'Star',
    items: [
      { path: '/dashboard', title: '工作概览', icon: 'Dashboard' },
      { path: '/invoice-ledger', title: '发票台账', icon: 'Connection' },
      { path: '/accounting', title: '应付与做账', icon: 'Coin' },
    ],
  },
  {
    key: 'grp-contract', title: '合同工作台', icon: 'FolderOpened',
    items: [
      { path: '/contracts', title: '合同台账', icon: 'Document' },
      { path: '/generate', title: 'AI 生成', icon: 'MagicStick' },
      { path: '/extraction', title: '任务中心', icon: 'UploadFilled' },
      { path: '/obligations', title: '义务看板', icon: 'List' },
      { path: '/contract-templates', title: '合同模板库', icon: 'Files' },
    ],
  },
  {
    key: 'grp-review', title: '审查与签署', icon: 'EditPen',
    items: [
      { path: '/chat', title: 'AI 助手', icon: 'ChatLineRound' },
      { path: '/files', title: '文件中心', icon: 'FolderOpened' },
      { path: '/query', title: '智能问答', icon: 'Search' },
      { path: '/approvals', title: '审批中心', icon: 'Stamp' },
      { path: '/party-profiles', title: '相对方画像', icon: 'DataAnalysis' },
      { path: '/sign', title: '签署管理', icon: 'EditPen' },
      { path: '/duban', title: '公文督办', icon: 'Memo' },
    ],
  },
  {
    key: 'grp-finance', title: '发票与财务', icon: 'Coin',
    items: [
      { path: '/invoices', title: '发票中心', icon: 'Tickets' },
      { path: '/invoice-class', title: '发票分类', icon: 'Collection' },
      { path: '/expense', title: '费控管理', icon: 'Wallet' },
      { path: '/skills', title: 'AI 技能', icon: 'MagicStick' },
      { path: '/revrec', title: '收入确认', icon: 'Coin' },
      { path: '/seals', title: '用印管理', icon: 'Stamp' },
      { path: '/archive-schedules', title: '归档调度', icon: 'Timer' },
      { path: '/long-term', title: '长期保存(国密)', icon: 'Lock' },
    ],
  },
  {
    key: 'grp-org', title: '组织与主数据', icon: 'OfficeBuilding',
    items: [
      { path: '/organization', title: '组织与主体', icon: 'OfficeBuilding' },
      { path: '/master-data', title: '主数据', icon: 'Files' },
      { path: '/mdm-dedup', title: '主数据去重', icon: 'Connection' },
    ],
  },
  {
    key: 'grp-governance', title: '治理与配置', icon: 'Setting',
    items: [
      { path: '/compliance', title: '合规策略', icon: 'CircleCheck' },
      { path: '/roles', title: '角色权限', icon: 'Lock' },
      { path: '/isolation', title: '强隔离策略', icon: 'Hide' },
      { path: '/identity-sync', title: '身份同步', icon: 'UserFilled' },
      { path: '/model-configs', title: '模型配置', icon: 'Setting' },
      { path: '/mcp-servers', title: 'MCP 客户端', icon: 'Connection' },
      { path: '/dengbao', title: '等保证据', icon: 'Stamp' },
      { path: '/audit-log', title: '审计日志', icon: 'Memo' },
      { path: '/api-docs', title: '接口文档', icon: 'Document' },
    ],
  },
  {
    key: 'grp-about', title: '关于我们', icon: 'InfoFilled',
    items: [
      { path: '/about', title: '关于我们', icon: 'InfoFilled' },
    ],
  },
]

// 当前路由属于哪个分组，用于进入页面时自动展开对应大条目
export function groupForPath(path: string): string {
  for (const g of menuGroups) {
    if (g.items.some((i) => i.path === path)) return g.key
  }
  return ''
}

// 菜单里展示的页面标题（router meta.title 的兜底，避免两处文案漂移）
export function titleForPath(path: string): string {
  for (const g of menuGroups) {
    const hit = g.items.find((i) => i.path === path)
    if (hit) return hit.title
  }
  return ''
}
