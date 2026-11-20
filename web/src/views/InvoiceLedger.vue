<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">发票台账（三方匹配 + 四性归档）</span>
        <span class="card-sub">P0 免费版核心：发票-合同-付款三方匹配 + 真实性/完整性/合规性/关联性四性归档审计</span>
      </div>
      <div class="toolbar-actions">
        <el-button type="primary" @click="runMatchSelected" :disabled="!selectedIds.length">匹配选中</el-button>
        <el-button type="success" @click="runMatchAll">全量匹配</el-button>
        <el-button @click="runAuditSelected" :disabled="!selectedIds.length">四性审计</el-button>
        <el-button type="primary" icon="Money" @click="oneClickPay" :disabled="!payableSelected.length">一键付款</el-button>
        <el-button type="warning" @click="exportLedger('xlsx')">导出 Excel</el-button>
        <el-button @click="exportLedger('md')">导出 Markdown</el-button>
      </div>
    </div>

    <el-table
      :data="ledger"
      v-loading="loading"
      border
      stripe
      size="small"
      highlight-current-row
      @selection-change="handleSelection"
      row-key="id"
    >
      <el-table-column type="selection" width="50" />
      <el-table-column prop="id" label="ID" width="56" />
      <el-table-column prop="invoice_no" label="发票号码" width="150" />
      <el-table-column prop="issue_date" label="开票日期" width="100" />
      <el-table-column prop="buyer_name" label="购买方" min-width="140" show-overflow-tooltip />
      <el-table-column prop="seller_name" label="销售方" min-width="140" show-overflow-tooltip />
      <el-table-column prop="seller_tax_id" label="销方税号" width="130" show-overflow-tooltip />
      <el-table-column prop="buyer_tax_id" label="购方税号" width="130" show-overflow-tooltip />
      <el-table-column label="价税合计" width="100">
        <template #default="{ row }">¥{{ fmt(row.total) }}</template>
      </el-table-column>
      <el-table-column label="验真" width="90">
        <template #default="{ row }">
          <el-tag v-if="row.verify_status === 'verified'" type="success" size="small">已验真</el-tag>
          <el-tag v-else-if="row.verify_status === 'failed'" type="danger" size="small">验真失败</el-tag>
          <el-tag v-else type="info" size="small">待验真</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="归档" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.archive_status === 'archived'" type="success" size="small">已归档</el-tag>
          <el-tag v-else type="info" size="small">未归档</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="匹配状态" width="110">
        <template #default="{ row }">
          <el-tag v-if="row.match_status === 'matched'" type="success" size="small">已匹配</el-tag>
          <el-tag v-else-if="row.match_status === 'mismatch'" type="danger" size="small">不匹配</el-tag>
          <el-tag v-else-if="row.match_status === 'pending'" type="warning" size="small">待确认</el-tag>
          <el-tag v-else type="info" size="small">未匹配</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="匹配分" width="80">
        <template #default="{ row }">{{ row.match_score || '—' }}</template>
      </el-table-column>
      <el-table-column label="关联合同" width="90">
        <template #default="{ row }">
          <span v-if="row.contract_id">#{{ row.contract_id }}</span>
          <span v-else class="dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="关联付款" width="90">
        <template #default="{ row }">
          <span v-if="row.payment_plan_id">#{{ row.payment_plan_id }}</span>
          <span v-else class="dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="160" fixed="right">
        <template #default="{ row }">
          <el-dropdown @command="handleRowCommand">
            <template #default>
              <el-button link size="small" type="primary" @click="showMatchDetail(row)">
                匹配详情
              </el-button>
            </template>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="audit" divided>
                  <el-icon><Edit /></el-icon>
                  四性审计
                </el-dropdown-item>
                <el-dropdown-item command="rematch" divided>
                  <el-icon><Refresh /></el-icon>
                  重新匹配
                </el-dropdown-item>
                <el-dropdown-item command="reaudit">
                  <el-icon><Delete /></el-icon>
                  重新审计
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
            <template #trigger="{ visible }">
              <el-button link size="small" :icon="More" @click="visible = !visible" />
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>

    <!-- 匹配详情抽屉 -->
    <el-drawer v-model="matchDrawer" :title="`匹配详情：发票 #${matchInvoiceId}`" size="60%" direction="rtl">
      <el-table v-if="matchLogs.length" :data="matchLogs" border size="small">
        <el-table-column prop="id" label="日志ID" width="70" />
        <el-table-column prop="contract_id" label="合同ID" width="80" />
        <el-table-column prop="payment_plan_id" label="付款计划ID" width="100" />
        <el-table-column prop="score_tax_id" label="税号分" width="70" />
        <el-table-column prop="score_name" label="名称分" width="70" />
        <el-table-column prop="score_amount" label="金额分" width="70" />
        <el-table-column prop="score_date" label="日期分" width="70" />
        <el-table-column prop="score_item" label="品目分" width="70" />
        <el-table-column prop="total_score" label="总分" width="70" />
        <el-table-column prop="match_status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.match_status === 'matched'" type="success" size="small">已匹配</el-tag>
            <el-tag v-else-if="row.match_status === 'mismatch'" type="danger" size="small">不匹配</el-tag>
            <el-tag v-else-if="row.match_status === 'pending'" type="warning" size="small">待确认</el-tag>
            <el-tag v-else type="info" size="small">未匹配</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="match_rule" label="规则" width="100" />
        <el-table-column prop="matched_by" label="来源" width="80" />
        <el-table-column prop="created_at" label="时间" width="160" />
      </el-table>
      <el-empty v-else description="暂无匹配记录" :image-size="80" />
    </el-drawer>

    <!-- 四性审计抽屉 -->
    <el-drawer v-model="auditDrawer" :title="`四性归档审计：发票 #${auditInvoiceId}`" size="60%" direction="rtl">
      <el-table v-if="auditLogs.length" :data="auditLogs" border size="small">
        <el-table-column prop="audit_type" label="审计类型" width="120">
          <template #default="{ row }">
            <el-tag :type="typeTag(row.audit_type)" size="small">{{ typeLabel(row.audit_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="field_name" label="检查字段" min-width="180" show-overflow-tooltip />
        <el-table-column label="结果" width="90">
          <template #default="{ row }">
            <el-tag :type="row.check_pass ? 'success' : 'danger'" size="small">{{ row.check_pass ? '通过' : '不通过' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="check_detail" label="详情" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <pre style="margin:0; font-size:11px; white-space:pre-wrap;">{{ formatDetail(row.check_detail) }}</pre>
          </template>
        </el-table-column>
        <el-table-column prop="operator" label="操作人" width="100" />
        <el-table-column prop="created_at" label="时间" width="160" />
      </el-table>
      <el-empty v-else description="暂无审计记录" :image-size="80" />
    </el-drawer>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type Row } from '../api'
import { ElMessage, ElLoading } from 'element-plus'
import { ElDropdown, ElDropdownMenu, ElDropdownItem, ElButton } from 'element-plus'
import { More, Edit, Delete, Refresh, Money } from '@element-plus/icons-vue'

const loading = ref(false)
let tableLoading: any = null
const ledger = ref<Row[]>([])
const selectedRows = ref<Row[]>([])
const selectedIds = ref<number[]>([])

const matchDrawer = ref(false)
const matchInvoiceId = ref<number | null>(null)
const matchLogs = ref<Row[]>([])

const auditDrawer = ref(false)
const auditInvoiceId = ref<number | null>(null)
const auditLogs = ref<Row[]>([])

// 一键付款：选中可付款的发票（已验真 + 已匹配）
const payableSelected = ref<Row[]>([])

const MATCH_STATUS_MAP: Record<string, string> = {
  matched: '已匹配',
  mismatch: '不匹配',
  pending: '待确认',
  unverified: '未匹配',
}

async function load() {
  loading.value = true
  try {
    const res = await api.invoiceLedger()
    if (res.code === 0) {
      ledger.value = res.data || []
      // 补全匹配状态字段用于显示
      for (const row of ledger.value) {
        row.match_status = row.match_status || 'unverified'
        row.match_score = row.match_score || 0
      }
    }
  } finally {
    loading.value = false
  }
}

function handleSelection(rows: Row[]) {
  selectedRows.value = rows
  selectedIds.value = rows.map(r => r.id)
  // 可付款：已验真、已匹配、未生成付款计划（payment_plan_id 为空 或已匹配但未付款）
  payableSelected.value = rows.filter(r =>
    r.verify_status === 'verified' &&
    (r.match_status === 'matched' || r.match_status === 'pending') &&
    !r.payment_plan_id
  )
}

// 三方匹配
async function runMatch(ids: number[], showToast = true) {
  if (!ids.length) return
  const target = tableLoading || ElLoading.service({
    lock: true,
    text: '处理中...',
    spinner: 'el-icon-loading',
    background: 'rgba(255, 255, 255, 0.7)'
  })
  try {
    const res = await api.invoiceMatchRun(ids)
    if (res.code === 0) {
      if (showToast) ElMessage.success(`匹配完成：${res.data?.length || 0} 笔`)
      await load()
    } else {
      ElMessage.error(res.message || '匹配失败')
    }
  } catch (e) {
    ElMessage.error('匹配请求失败')
  } finally {
    target.close()
  }
}

async function runMatchSelected() {
  await runMatch(selectedIds.value)
}

async function runMatchAll() {
  const allIds = ledger.value.map(r => r.id)
  await runMatch(allIds)
}

async function runSingleMatch(row: Row) {
  await runMatch([row.id])
}

// 四性审计
async function runAudit(ids: number[]) {
  if (!ids.length) return
  const target = ElLoading.service({
    lock: true,
    text: '审计中...',
    spinner: 'el-icon-loading',
    background: 'rgba(255, 255, 255, 0.7)'
  })
  let failed = 0
  for (const id of ids) {
    const res = await api.invoiceAuditRun(id)
    if (res.code !== 0) {
      failed++
      ElMessage.error(`发票 #${id} 审计失败：${res.message}`)
    }
  }
  target.close()
  if (failed === 0) {
    ElMessage.success(`四性审计完成：${ids.length} 笔`)
    await load()
  } else {
    ElMessage.warning(`四性审计完成：成功 ${ids.length - failed} 笔，失败 ${failed} 笔`)
    await load()
  }
}

async function runAuditSelected() {
  await runAudit(selectedIds.value)
}

async function runSingleAudit(row: Row) {
  await runAudit([row.id])
}

// 一键全流程：选中发票 → 生成付款计划 → 跳转应付台账打款
async function oneClickPay() {
  if (!payableSelected.value.length) {
    ElMessage.warning('请先勾选已验真、已匹配且未生成付款计划的发票')
    return
  }
  const ids = payableSelected.value.map(r => r.id)
  const target = ElLoading.service({
    lock: true,
    text: '生成付款计划中...',
    spinner: 'el-icon-loading',
    background: 'rgba(255, 255, 255, 0.7)'
  })
  try {
    const res = await api.payableGenerateFromInvoices({ invoiceIds: ids, days: 30 })
    if (res.code === 0) {
      ElMessage.success(`已为 ${res.data?.created || 0} 笔发票生成付款计划，跳转至应付台账登记打款`)
      location.hash = '#/accounting'
    } else {
      ElMessage.error(res.message || '生成付款计划失败')
    }
  } catch (e) {
    ElMessage.error('生成付款计划请求失败')
  } finally {
    target.close()
  }
}

// 下拉菜单命令处理
function handleRowCommand(command: string, row: Row) {
  switch (command) {
    case 'audit':
      showAuditDetail(row)
      break
    case 'rematch':
      runSingleMatch(row)
      break
    case 'reaudit':
      runSingleAudit(row)
      break
  }
}

// 匹配详情
async function showMatchDetail(row: Row) {
  matchInvoiceId.value = row.id
  const res = await api.invoiceMatchLogs(row.id)
  matchLogs.value = res.code === 0 ? (res.data || []) : []
  matchDrawer.value = true
}

// 四性审计详情
async function showAuditDetail(row: Row) {
  auditInvoiceId.value = row.id
  const res = await api.invoiceAuditLogs(row.id)
  auditLogs.value = res.code === 0 ? (res.data || []) : []
  auditDrawer.value = true
}

// 导出台账
function exportLedger(format: 'xlsx' | 'md') {
  if (!selectedIds.value.length) {
    ElMessage.warning('请先勾选要导出的发票')
    return
  }
  window.open(api.invoiceLedgerExportUrl(selectedIds.value, format), '_blank')
}

function typeTag(t: string) {
  const m: Record<string, 'success' | 'warning' | 'info' | 'danger'> = {
    authenticity: 'success',
    integrity: 'info',
    compliance: 'warning',
    association: 'danger',
  }
  return m[t] || 'info'
}

function typeLabel(t: string) {
  const m: Record<string, string> = {
    authenticity: '真实性',
    integrity: '完整性',
    compliance: '合规性',
    association: '关联性',
  }
  return m[t] || t
}

function formatDetail(detail: any) {
  if (!detail) return ''
  return JSON.stringify(detail, null, 2)
}

function fmt(n: number) {
  return Number(n || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2 })
}

onMounted(load)
</script>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
  flex-wrap: wrap;
  gap: 10px;
}
.card-title {
  font-size: 16px;
  font-weight: 600;
  margin-right: 12px;
}
.card-sub {
  font-size: 12px;
  color: #909399;
}
.toolbar-actions {
  display: flex;
  gap: 8px;
}
.dim {
  color: #c0c4cc;
}
</style>