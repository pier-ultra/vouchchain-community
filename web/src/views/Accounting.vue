<template>
  <div class="accounting">
    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="业务侧 → 账 的自动化桥梁（社区版免费，无企业版/license 分层）"
      description="合同义务 → 收票验真 → AI 分录建议 → 会计确认入账 → 挂应付 → 按期打款 → 回单销账。不做总账本体、报表、报税。"
      style="margin-bottom: 12px"
    />
    <el-tabs v-model="tab">
      <!-- ============ 应付台账 ============ -->
      <el-tab-pane label="应付台账" name="payables">
        <el-row :gutter="12" class="stat-row">
          <el-col :span="8">
            <el-card shadow="never"><div class="stat-label">待付金额</div><div class="stat-num">¥ {{ fmt(summary.plannedAmount) }}</div></el-card>
          </el-col>
          <el-col :span="8">
            <el-card shadow="never"><div class="stat-label">逾期笔数</div><div class="stat-num danger">{{ summary.overdueCount }}</div></el-card>
          </el-col>
          <el-col :span="8">
            <el-card shadow="never"><div class="stat-label">已付金额</div><div class="stat-num">¥ {{ fmt(summary.paidAmount) }}</div></el-card>
          </el-col>
        </el-row>
        <div class="bar">
          <span>账期</span>
          <el-input-number v-model="days" :min="1" :max="180" size="small" controls-position="right" />
          <span>天</span>
          <el-checkbox v-model="onlyVerified" style="margin: 0 12px">只收已验真发票</el-checkbox>
          <el-button type="primary" size="small" :loading="loading.gen" @click="genPlans">从发票生成付款计划</el-button>
          <el-button size="small" @click="loadPayables">刷新</el-button>
        </div>
        <el-table :data="payables" border size="small" v-loading="loading.payables">
          <el-table-column prop="invoiceNo" label="发票号" width="160" />
          <el-table-column prop="payee" label="收款方" min-width="140" show-overflow-tooltip />
          <el-table-column label="金额" width="120">
            <template #default="{ row }">¥ {{ fmt(row.amount) }}</template>
          </el-table-column>
          <el-table-column label="到期日" width="120">
            <template #default="{ row }">
              <span :class="{ danger: row.overdue }">{{ row.dueDate || '—' }}</span>
              <el-tag v-if="row.overdue" type="danger" size="small" style="margin-left: 4px">逾期</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="验真" width="90">
            <template #default="{ row }">
              <el-tag :type="row.verifyStatus === 'verified' ? 'success' : 'info'" size="small">
                {{ row.verifyStatus === 'verified' ? '已验真' : '未验真' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'paid' ? 'success' : row.status === 'cancelled' ? 'info' : 'warning'" size="small">
                {{ { planned: '待付款', paid: '已付款', cancelled: '已取消' }[row.status] || row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" :disabled="row.status !== 'planned'" @click="openPay(row)">登记打款</el-button>
              <el-button link size="small" :disabled="row.status !== 'planned'" @click="openDue(row)">改账期</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- ============ AI 分录草稿 ============ -->
      <el-tab-pane label="AI 分录草稿" name="journal">
        <div class="bar">
          <el-select v-model="draftForm.invoiceId" placeholder="选择进项发票" style="width: 220px" size="small" filterable>
            <el-option v-for="i in invoices" :key="i.id" :label="`${i.invoice_no || '#' + i.id} · ¥${fmt(Number(i.total))}`" :value="i.id" />
          </el-select>
          <el-select v-model="draftForm.category" placeholder="费用类型" style="width: 150px" size="small">
            <el-option v-for="c in expenseCategories" :key="c" :label="c" :value="c" />
          </el-select>
          <el-radio-group v-model="draftForm.mode" size="small">
            <el-radio-button value="ai">AI 建议</el-radio-button>
            <el-radio-button value="rule">规则兜底</el-radio-button>
          </el-radio-group>
          <el-button type="primary" size="small" :loading="loading.draft" @click="createDraft">生成分录草稿</el-button>
          <el-button size="small" @click="loadDrafts">刷新</el-button>
        </div>
        <el-alert v-if="suggestion" :type="suggestion.source === 'llm' ? 'success' : 'warning'" :closable="false" show-icon
          :title="`来源：${{ llm: 'AI 建议', rule: '规则兜底', rule_fallback: 'AI 不可用/输出不合规，已回退规则' }[suggestion.source] || suggestion.source}`"
          :description="suggestion.reason" style="margin-bottom: 12px" />
        <el-table :data="drafts" border size="small" v-loading="loading.drafts">
          <el-table-column prop="id" label="#" width="60" />
          <el-table-column prop="invoiceNo" label="发票号" width="150" />
          <el-table-column prop="category" label="费用类型" width="110" />
          <el-table-column label="金额" width="110">
            <template #default="{ row }">¥ {{ fmt(row.total) }}</template>
          </el-table-column>
          <el-table-column prop="reason" label="判断依据" min-width="220" show-overflow-tooltip />
          <el-table-column label="状态" width="110">
            <template #default="{ row }">
              <el-tag :type="row.status === 'confirmed' ? 'success' : 'warning'" size="small">
                {{ row.status === 'confirmed' ? `已入账 #${row.voucherId}` : '待会计确认' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" :disabled="row.status !== 'draft'" @click="openDraft(row)">复核 / 确认</el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-collapse style="margin-top: 16px">
          <el-collapse-item title="科目映射维护（AI 只能从这张表选科目，禁止编造）" name="mapping">
            <div class="bar">
              <el-input v-model="mapForm.category" placeholder="费用类型" size="small" style="width: 140px" />
              <el-input v-model="mapForm.subjectCode" placeholder="科目编码" size="small" style="width: 140px" />
              <el-input v-model="mapForm.subjectName" placeholder="科目名称" size="small" style="width: 160px" />
              <el-select v-model="mapForm.direction" size="small" style="width: 100px">
                <el-option label="借方" value="debit" /><el-option label="贷方" value="credit" />
              </el-select>
              <el-button size="small" type="primary" @click="saveMapping">保存映射</el-button>
            </div>
            <el-table :data="mappings" border size="small">
              <el-table-column prop="category" label="费用类型" />
              <el-table-column prop="subject_code" label="科目编码" />
              <el-table-column prop="subject_name" label="科目名称" />
              <el-table-column prop="direction" label="方向" width="90" />
            </el-table>
          </el-collapse-item>
        </el-collapse>
      </el-tab-pane>


      <!-- ============ 审计与归档（AI5.5 / AK4.4-4.5） ============ -->
      <el-tab-pane label="审计与归档" name="archive">
        <div class="bar">
          <el-input v-model="archiveQuery.keyword" placeholder="发票号 / 销方" size="small" style="width: 200px" clearable />
          <el-select v-model="archiveQuery.verifyStatus" size="small" style="width: 130px" clearable placeholder="验真状态">
            <el-option label="已验真" value="verified" /><el-option label="未验真" value="unverified" />
          </el-select>
          <el-input v-model="archiveQuery.dateFrom" placeholder="开票日期起 YYYY-MM-DD" size="small" style="width: 200px" clearable />
          <el-button size="small" type="primary" @click="loadArchives">检索归档</el-button>
          <el-button size="small" @click="loadArchives">刷新</el-button>
        </div>
        <el-table :data="archives" border size="small">
          <el-table-column prop="invoiceNo" label="发票号" width="170" />
          <el-table-column prop="sellerName" label="销售方" min-width="150" show-overflow-tooltip />
          <el-table-column prop="issueDate" label="开票日期" width="120" />
          <el-table-column label="价税合计" width="120">
            <template #default="{ row }">¥ {{ fmt(row.total) }}</template>
          </el-table-column>
          <el-table-column prop="verifyStatus" label="验真" width="100" />
          <el-table-column label="归档包" width="100">
            <template #default="{ row }"><a :href="api.archiveDownloadUrl(row.id)" target="_blank">下载 zip</a></template>
          </el-table-column>
        </el-table>
        <el-divider />
        <div class="bar">
          <el-input v-model="auditQuery.action" placeholder="动作筛选（如 payable / journal / bridge）" size="small" style="width: 260px" clearable />
          <el-button size="small" type="primary" @click="loadAudit">查审计轨迹</el-button>
          <el-button size="small" @click="exportAudit">导出审计 Excel</el-button>
        </div>
        <el-table :data="auditLogs" border size="small">
          <el-table-column prop="createdAt" label="时间" width="170" />
          <el-table-column prop="username" label="用户" width="90" />
          <el-table-column prop="action" label="动作" min-width="150" />
          <el-table-column prop="targetType" label="对象" width="110" />
          <el-table-column prop="ip" label="来源 IP" width="130" />
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 登记打款 -->
    <el-dialog v-model="payVisible" title="登记打款（自动写回单并销账）" width="440px">
      <el-form label-width="90px">
        <el-form-item label="打款日期"><el-date-picker v-model="payForm.paidDate" type="date" value-format="YYYY-MM-DD" style="width: 100%" /></el-form-item>
        <el-form-item label="付款方"><el-input v-model="payForm.payer" placeholder="默认取发票购买方" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="payForm.remark" type="textarea" :rows="2" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="payVisible = false">取消</el-button><el-button type="primary" @click="doPay">确认打款</el-button></template>
    </el-dialog>

    <!-- 改账期 -->
    <el-dialog v-model="dueVisible" title="调整账期" width="380px">
      <el-date-picker v-model="dueDate" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
      <template #footer><el-button @click="dueVisible = false">取消</el-button><el-button type="primary" @click="doUpdateDue">保存</el-button></template>
    </el-dialog>

    <!-- 会计复核分录 -->
    <el-drawer v-model="drawerVisible" title="会计复核分录（确认后才写正式凭证）" size="62%">
      <el-alert type="info" :closable="false" :description="currentDraft?.reason || ''" style="margin-bottom: 12px" />
      <el-table :data="editEntries" border size="small">
        <el-table-column label="摘要" min-width="160">
          <template #default="{ row }"><el-input v-model="row.summary" size="small" /></template>
        </el-table-column>
        <el-table-column label="科目编码" width="130">
          <template #default="{ row }"><el-input v-model="row.subjectCode" size="small" /></template>
        </el-table-column>
        <el-table-column label="科目名称" width="150">
          <template #default="{ row }"><el-input v-model="row.subjectName" size="small" /></template>
        </el-table-column>
        <el-table-column label="借方" width="130">
          <template #default="{ row }"><el-input-number v-model="row.debit" :min="0" :precision="2" size="small" controls-position="right" style="width: 100%" /></template>
        </el-table-column>
        <el-table-column label="贷方" width="130">
          <template #default="{ row }"><el-input-number v-model="row.credit" :min="0" :precision="2" size="small" controls-position="right" style="width: 100%" /></template>
        </el-table-column>
      </el-table>
      <div class="balance-bar">
        借方合计 <b>¥ {{ fmt(debitSum) }}</b> ／ 贷方合计 <b>¥ {{ fmt(creditSum) }}</b>
        <el-tag :type="balanced ? 'success' : 'danger'" size="small" style="margin-left: 12px">{{ balanced ? '借贷平衡' : '借贷不平，禁止入账' }}</el-tag>
      </div>
      <div style="margin-top: 16px">
        <el-button @click="saveDraft" :disabled="!balanced">保存修改</el-button>
        <el-button type="primary" :disabled="!balanced" @click="confirmDraft">确认入账（写正式凭证 + 关联发票）</el-button>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  api,
  type PayableRow, type PayableSummary, type JournalDraftRow, type JournalEntry,
  type AccountMappingRow, type InvoiceRow, type VoucherRow,
  type ArchiveRow, type AuditLogRow,
} from '../api'

const tab = ref('payables')
const fmt = (n: number) => (Number(n) || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const loading = reactive({ gen: false, payables: false, draft: false, drafts: false })

// ---------- 应付台账 ----------
const payables = ref<PayableRow[]>([])
const summary = ref<PayableSummary>({ plannedAmount: 0, paidAmount: 0, overdueCount: 0 })
const days = ref(30)
const onlyVerified = ref(true)

async function loadPayables() {
  loading.payables = true
  try {
    const res = await api.payables()
    payables.value = res.data?.list || []
    summary.value = res.data?.summary || { plannedAmount: 0, paidAmount: 0, overdueCount: 0 }
  } finally { loading.payables = false }
}
async function genPlans() {
  loading.gen = true
  try {
    const res = await api.payableGenerate({ days: days.value, onlyVerified: onlyVerified.value })
    ElMessage.success(`新建 ${res.data?.created || 0} 条，跳过 ${res.data?.skipped || 0} 条（已建计划 / 未验真）`)
    await loadPayables()
  } finally { loading.gen = false }
}
const payVisible = ref(false)
const payForm = reactive({ paidDate: '', payer: '', remark: '' })
let payId = 0
function openPay(row: PayableRow) { payId = row.id; payVisible.value = true }
async function doPay() {
  await api.payablePay(payId, payForm)
  ElMessage.success('已登记打款，银行回单自动关联该发票')
  payVisible.value = false
  await loadPayables()
}
const dueVisible = ref(false)
const dueDate = ref('')
let dueId = 0
function openDue(row: PayableRow) { dueId = row.id; dueDate.value = row.dueDate; dueVisible.value = true }
async function doUpdateDue() {
  await api.payableUpdate(dueId, { dueDate: dueDate.value })
  ElMessage.success('账期已更新')
  dueVisible.value = false
  await loadPayables()
}

// ---------- AI 分录草稿 ----------
const invoices = ref<InvoiceRow[]>([])
const mappings = ref<AccountMappingRow[]>([])
const drafts = ref<JournalDraftRow[]>([])
const draftForm = reactive({ invoiceId: undefined as number | undefined, category: '服务采购', mode: 'ai' })
const suggestion = ref<{ source: string; reason: string } | null>(null)
const expenseCategories = computed(() => mappings.value
  .filter(m => m.direction === 'debit' && m.category !== '进项税额')
  .map(m => m.category))
async function loadRefs() {
  const [iv, mp] = await Promise.all([api.invoices(), api.accountMappings()])
  invoices.value = iv.data || []
  mappings.value = mp.data || []
}
async function loadDrafts() {
  loading.drafts = true
  try { drafts.value = (await api.journalDrafts()).data || [] } finally { loading.drafts = false }
}
async function createDraft() {
  if (!draftForm.invoiceId) { ElMessage.warning('请先选择发票'); return }
  loading.draft = true
  try {
    const res = await api.journalDraftCreate({
      invoiceId: draftForm.invoiceId, category: draftForm.category, mode: draftForm.mode,
    })
    suggestion.value = { source: res.data?.source || '', reason: res.data?.reason || '' }
    ElMessage.success('分录草稿已生成，待会计复核确认')
    await loadDrafts()
  } finally { loading.draft = false }
}
const mapForm = reactive({ category: '', subjectCode: '', subjectName: '', direction: 'debit' })
async function saveMapping() {
  if (!mapForm.category || !mapForm.subjectCode) { ElMessage.warning('费用类型与科目编码必填'); return }
  await api.accountMappingSave({ ...mapForm })
  ElMessage.success('科目映射已保存')
  await loadRefs()
}
const drawerVisible = ref(false)
const currentDraft = ref<JournalDraftRow | null>(null)
const editEntries = ref<JournalEntry[]>([])
const debitSum = computed(() => editEntries.value.reduce((s, e) => s + Number(e.debit || 0), 0))
const creditSum = computed(() => editEntries.value.reduce((s, e) => s + Number(e.credit || 0), 0))
const balanced = computed(() => editEntries.value.length >= 2 && Math.abs(debitSum.value - creditSum.value) < 0.01 && debitSum.value > 0
  && editEntries.value.every(e => Number(e.debit) >= 0 && Number(e.credit) >= 0 && !(Number(e.debit) > 0 && Number(e.credit) > 0)))
function openDraft(row: JournalDraftRow) {
  currentDraft.value = row
  try { editEntries.value = JSON.parse(row.entries) as JournalEntry[] } catch { editEntries.value = [] }
  drawerVisible.value = true
}
async function saveDraft() {
  if (!currentDraft.value) return
  await api.journalDraftUpdate(currentDraft.value.id, editEntries.value)
  ElMessage.success('分录已保存（仍需借贷平衡）')
  await loadDrafts()
}
async function confirmDraft() {
  if (!currentDraft.value) return
  if (editEntries.value.length) {
    await api.journalDraftUpdate(currentDraft.value.id, editEntries.value)
  }
  const res = await api.journalDraftConfirm(currentDraft.value.id)
  ElMessage.success(`已入账：${res.data?.voucherNo}（凭证 #${res.data?.voucherId}）`)
  drawerVisible.value = false
  await Promise.all([loadDrafts(), loadRefs()])
}

// ---------- 审计与归档（AI5.5 / AK4.4-4.5） ----------
const archives = ref<ArchiveRow[]>([])
const auditLogs = ref<AuditLogRow[]>([])
const archiveQuery = reactive({ keyword: '', verifyStatus: '', dateFrom: '' })
const auditQuery = reactive({ action: '', targetType: '', dateFrom: '' })
async function loadArchives() {
  archives.value = (await api.archives({
    keyword: archiveQuery.keyword || undefined,
    verifyStatus: archiveQuery.verifyStatus || undefined,
    dateFrom: archiveQuery.dateFrom || undefined,
  })).data || []
}
async function loadAudit() {
  auditLogs.value = (await api.auditLogs({
    action: auditQuery.action || undefined,
    targetType: auditQuery.targetType || undefined,
    dateFrom: auditQuery.dateFrom || undefined,
  })).data || []
}
function exportAudit() {
  window.open(api.auditLogExportUrl({ ...auditQuery }), '_blank')
}


onMounted(async () => {
  await Promise.all([loadPayables(), loadRefs(), loadDrafts(), loadArchives(), loadAudit()])
})
</script>

<style scoped>
.bar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
.stat-row { margin-bottom: 12px; }
.stat-label { color: #909399; font-size: 13px; }
.stat-num { font-size: 22px; font-weight: 600; margin-top: 6px; }
.danger { color: #f56c6c; }
.balance-bar { margin-top: 12px; font-size: 14px; }
a { color: #409eff; text-decoration: none; }
</style>
