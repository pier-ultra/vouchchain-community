import { createRouter, createWebHashHistory } from 'vue-router'
import MainLayout from '../layout/MainLayout.vue'

// 用 hash 路由：单二进制内嵌 SPA 时无需服务端 history 回退配置
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/login', component: () => import('../views/Login.vue') },
    {
      path: '/',
      component: MainLayout,
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { title: '工作概览' } },
        { path: 'contracts', component: () => import('../views/Contracts.vue'), meta: { title: '合同台账' } },
        { path: 'generate', component: () => import('../views/Generate.vue'), meta: { title: 'AI 生成' } },
        { path: 'extraction', component: () => import('../views/Extraction.vue'), meta: { title: '任务中心' } },
        { path: 'obligations', component: () => import('../views/Obligations.vue'), meta: { title: '义务看板' } },
        { path: 'contract-templates', component: () => import('../views/ContractTemplates.vue'), meta: { title: '合同模板库' } },
        { path: 'query', component: () => import('../views/Query.vue'), meta: { title: '智能问答' } },
        { path: 'sign', component: () => import('../views/Sign.vue'), meta: { title: '签署管理' } },
        { path: 'approvals', component: () => import('../views/Approvals.vue'), meta: { title: '审批中心' } },
        { path: 'invoices', component: () => import('../views/InvoiceCenter.vue'), meta: { title: '发票中心' } },
        { path: 'invoice-ledger', component: () => import('../views/InvoiceLedger.vue'), meta: { title: '发票台账（三方匹配+四性归档）' } },
        { path: 'accounting', component: () => import('../views/Accounting.vue'), meta: { title: '应付与做账' } },
        { path: 'model-configs', component: () => import('../views/ModelConfigs.vue'), meta: { title: '模型配置' } },
        { path: 'organization', component: () => import('../views/Organization.vue'), meta: { title: '组织与主体' } },
        { path: 'master-data', component: () => import('../views/MasterData.vue'), meta: { title: '主数据' } },
        { path: 'archive-schedules', component: () => import('../views/ArchiveSchedule.vue'), meta: { title: '归档调度' } },
        { path: 'long-term', component: () => import('../views/LongTerm.vue'), meta: { title: '长期保存(国密)' } },
        { path: 'compliance', component: () => import('../views/Compliance.vue'), meta: { title: '合规策略' } },
        { path: 'mdm-dedup', component: () => import('../views/MdmDedup.vue'), meta: { title: '主数据去重' } },
        { path: 'roles', component: () => import('../views/Roles.vue'), meta: { title: '角色权限' } },
        { path: 'chat', component: () => import('../views/Chat.vue'), meta: { title: 'AI 助手' } },
        { path: 'files', component: () => import('../views/FileCenter.vue'), meta: { title: '文件中心' } },
        { path: 'invoice-class', component: () => import('../views/InvoiceClass.vue'), meta: { title: '发票分类' } },
        { path: 'expense', component: () => import('../views/ExpenseControl.vue'), meta: { title: '费控管理' } },
        { path: 'skills', component: () => import('../views/Skills.vue'), meta: { title: 'AI 技能' } },
        { path: 'revrec', component: () => import('../views/RevRec.vue'), meta: { title: '收入确认' } },
        { path: 'seals', component: () => import('../views/Seals.vue'), meta: { title: '用印管理' } },
        { path: 'isolation', component: () => import('../views/Isolation.vue'), meta: { title: '强隔离策略' } },
        { path: 'identity-sync', component: () => import('../views/IdentitySync.vue'), meta: { title: '身份同步' } },
        { path: 'party-profiles', component: () => import('../views/PartyProfile.vue'), meta: { title: '相对方画像' } },
        { path: 'dengbao', component: () => import('../views/Dengbao.vue'), meta: { title: '等保证据' } },
        { path: 'duban', component: () => import('../views/Duban.vue'), meta: { title: '公文督办' } },
        { path: 'audit-log', component: () => import('../views/AuditLog.vue'), meta: { title: '审计日志' } },
        { path: 'api-docs', component: () => import('../views/ApiDocs.vue'), meta: { title: '接口文档' } },
        { path: 'about', component: () => import('../views/AboutUs.vue'), meta: { title: '关于我们' } },
      ],
    },
  ],
})

router.beforeEach((to) => {
  if (to.path !== '/login' && !localStorage.getItem('ch_token')) {
    return '/login'
  }
})

export default router
