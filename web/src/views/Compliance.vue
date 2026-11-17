<template>
  <div>
    <el-alert type="info" :closable="false" show-icon
      title="合规策略引擎（产品② US-G4 / AG4.1–AG4.6）"
      description="制度条款落成可执行规则：必填 / 格式 / 关联 / 阈值四类，可绑定到对象子类型与状态。业务写入时实时校验，block 级直接拦截并留痕，warn 级告警记录待人工处理；规则编辑自动升版本并保留历史快照。" />

    <el-row :gutter="12" class="stats">
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">规则总数</div><div class="stat-value">{{ summary.total }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">启用中</div><div class="stat-value">{{ summary.enabled }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">待处理违规</div><div class="stat-value warn">{{ summary.open }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">累计拦截</div><div class="stat-value danger">{{ summary.blocked }}</div></el-card></el-col>
    </el-row>

    <el-tabs v-model="tab" class="tabs">
      <!-- AG4.1/4.2/4.3/4.6 规则库 -->
      <el-tab-pane label="规则库" name="rules">
        <div class="toolbar">
          <el-button type="primary" @click="openRule()">新增规则</el-button>
          <el-button @click="loadRules">刷新</el-button>
          <span class="hint">规则变更立即生效；编辑会升版本并留旧快照。</span>
        </div>
        <el-table :data="rules" border stripe v-loading="loading" size="small">
          <el-table-column prop="code" label="编码" width="170" />
          <el-table-column prop="name" label="名称" min-width="150" />
          <el-table-column label="类型" width="95">
            <template #default="{ row }"><el-tag size="small">{{ typeLabel(row.rule_type) }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="object_type" label="业务对象" width="110" />
          <el-table-column prop="field" label="字段" width="120" />
          <el-table-column label="条件/值" min-width="150">
            <template #default="{ row }">
              <span>{{ row.op }} {{ row.rule_value }}</span>
              <el-tag v-if="row.condition_expr" size="small" type="warning" style="margin-left: 6px">{{ row.condition_expr }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="处置" width="85">
            <template #default="{ row }">
              <el-tag size="small" :type="row.severity === 'block' ? 'danger' : 'warning'">{{ row.severity === 'block' ? '拦截' : '告警' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="version" label="版本" width="65" />
          <el-table-column label="状态" width="80">
            <template #default="{ row }"><el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="操作" width="230" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openRule(row)">编辑</el-button>
              <el-button link type="primary" @click="toggleRule(row)">{{ row.enabled ? '停用' : '启用' }}</el-button>
              <el-button link type="info" @click="showHistory(row)">历史</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- AG4.5 违规记录 -->
      <el-tab-pane label="违规记录" name="violations">
        <div class="toolbar">
          <el-select v-model="vStatus" style="width: 150px" @change="loadViolations">
            <el-option label="全部状态" value="" />
            <el-option label="待处理" value="open" />
            <el-option label="已整改" value="resolved" />
            <el-option label="已豁免" value="waived" />
          </el-select>
          <el-select v-model="vSeverity" style="width: 140px" @change="loadViolations">
            <el-option label="全部级别" value="" />
            <el-option label="拦截" value="block" />
            <el-option label="告警" value="warn" />
          </el-select>
          <el-button @click="loadViolations">刷新</el-button>
          <span class="hint">拦截与告警都会留痕；处理时必须填说明。</span>
        </div>
        <el-table :data="violations" border stripe size="small">
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column label="级别" width="75">
            <template #default="{ row }"><el-tag size="small" :type="row.severity === 'block' ? 'danger' : 'warning'">{{ row.severity === 'block' ? '拦截' : '告警' }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="rule_name" label="命中规则" min-width="150" />
          <el-table-column prop="object_label" label="业务对象" min-width="130" />
          <el-table-column prop="message" label="违规说明" min-width="240" show-overflow-tooltip />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.status === 'open' ? 'danger' : 'success'">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" width="160">
            <template #default="{ row }">{{ fmt(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <el-button v-if="row.status === 'open'" link type="primary" @click="openHandle(row)">处理</el-button>
              <span v-else class="hint">{{ row.handled_by }}</span>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- AG4.4 手动试跑 -->
      <el-tab-pane label="规则试跑" name="check">
        <div class="toolbar">
          <el-select v-model="checkType" style="width: 170px">
            <el-option label="合同 contract" value="contract" />
            <el-option label="发票 invoice" value="invoice" />
            <el-option label="凭证 voucher" value="voucher" />
            <el-option label="主数据 master_data" value="master_data" />
          </el-select>
          <el-button type="primary" @click="runCheck">执行校验</el-button>
          <span class="hint">试跑不落库、不拦截，用来确认规则配置是否符合预期。</span>
        </div>
        <el-input v-model="checkJson" type="textarea" :rows="5" placeholder="业务记录 JSON" />
        <el-table v-if="checkResults.length" :data="checkResults" border stripe size="small" style="margin-top: 12px">
          <el-table-column prop="ruleCode" label="规则" width="180" />
          <el-table-column prop="ruleName" label="名称" min-width="150" />
          <el-table-column label="结论" width="90">
            <template #default="{ row }"><el-tag size="small" :type="row.passed ? 'success' : 'danger'">{{ row.passed ? '通过' : '不通过' }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="message" label="说明" min-width="240" />
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="ruleDialog" :title="form.id ? '编辑规则（将升版本）' : '新增规则'" width="620px">
      <el-form :model="form" label-width="110px">
        <el-form-item label="规则编码"><el-input v-model="form.code" :disabled="!!form.id" placeholder="如 CR-CONTRACT-NEW" /></el-form-item>
        <el-form-item label="规则名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="规则类型">
          <el-select v-model="form.ruleType" style="width: 100%">
            <el-option label="必填校验 required" value="required" />
            <el-option label="格式校验 format" value="format" />
            <el-option label="关联校验 relation" value="relation" />
            <el-option label="阈值校验 threshold" value="threshold" />
          </el-select>
        </el-form-item>
        <el-form-item label="业务对象">
          <el-select v-model="form.objectType" style="width: 100%">
            <el-option v-for="o in ['contract', 'invoice', 'voucher', 'master_data']" :key="o" :label="o" :value="o" />
          </el-select>
        </el-form-item>
        <el-form-item label="校验字段"><el-input v-model="form.field" placeholder="如 counterParty / amount / taxNo" /></el-form-item>
        <el-form-item label="操作符"><el-input v-model="form.op" placeholder="threshold: gt/gte/lt/lte/eq；format: regex；relation: exists" /></el-form-item>
        <el-form-item label="值"><el-input v-model="form.ruleValue" placeholder="阈值数字 / 正则表达式 / 被关联字段" /></el-form-item>
        <el-form-item label="绑定条件"><el-input v-model="form.conditionExpr" placeholder="可选，如 mdType=supplier / verifyStatus=verified" /></el-form-item>
        <el-form-item label="处置方式">
          <el-radio-group v-model="form.severity">
            <el-radio value="block">拦截（block）</el-radio>
            <el-radio value="warn">告警（warn）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="制度依据"><el-input v-model="form.doc" type="textarea" :rows="2" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="handleDialog" title="处理违规记录" width="480px">
      <div class="hint" style="margin-bottom: 10px">规则：{{ current?.rule_name }}｜说明：{{ current?.message }}</div>
      <el-radio-group v-model="handleAction" style="margin-bottom: 10px">
        <el-radio value="resolved">已整改</el-radio>
        <el-radio value="waived">豁免</el-radio>
      </el-radio-group>
      <el-input v-model="handleNote" type="textarea" :rows="3" placeholder="处理说明（必填，写入审计）" />
      <template #footer>
        <el-button @click="handleDialog = false">取消</el-button>
        <el-button type="primary" @click="submitHandle">提交</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="historyDialog" title="规则版本历史" width="620px">
      <el-table :data="histories" border size="small">
        <el-table-column prop="version" label="版本" width="80" />
        <el-table-column prop="changed_by" label="修改人" width="110" />
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ fmt(row.changed_at) }}</template>
        </el-table-column>
        <el-table-column prop="snapshot" label="旧快照（JSON）" show-overflow-tooltip />
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { govApi, type ComplianceRuleRow, type ComplianceViolationRow, type ComplianceCheckResult } from '../api'

const tab = ref('rules')
const loading = ref(false)
const rules = ref<ComplianceRuleRow[]>([])
const violations = ref<ComplianceViolationRow[]>([])
const checkResults = ref<ComplianceCheckResult[]>([])
const summary = reactive({ total: 0, enabled: 0, open: 0, blocked: 0 })
const vStatus = ref('')
const vSeverity = ref('')
const checkType = ref('contract')
const checkJson = ref('{\n  "title": "试跑合同",\n  "counterParty": "甲公司",\n  "amount": 2000000,\n  "status": "draft"\n}')

const ruleDialog = ref(false)
const form = reactive({
  id: 0, code: '', name: '', ruleType: 'required', objectType: 'contract', field: '',
  op: '', ruleValue: '', conditionExpr: '', severity: 'warn', doc: '', description: '',
})
const handleDialog = ref(false)
const handleAction = ref<'resolved' | 'waived'>('resolved')
const handleNote = ref('')
const current = ref<ComplianceViolationRow | null>(null)
const historyDialog = ref(false)
const histories = ref<{ version: number; snapshot: string; changed_by: string; changed_at: string }[]>([])

function fmt(v?: string) { return v ? String(v).replace('T', ' ').slice(0, 19) : '-' }
function typeLabel(t: string) {
  return ({ required: '必填', format: '格式', relation: '关联', threshold: '阈值' } as Record<string, string>)[t] || t
}
function statusLabel(s: string) {
  return ({ open: '待处理', resolved: '已整改', waived: '已豁免' } as Record<string, string>)[s] || s
}

async function loadAll() { await Promise.all([loadRules(), loadViolations(), loadSummary()]) }
async function loadSummary() {
  const res = await govApi.complianceSummary()
  if (!res.data) return
  // 逐属性赋值，确保响应式追踪到每个字段
  summary.total = Number(res.data.total ?? 0)
  summary.enabled = Number(res.data.enabled ?? 0)
  summary.open = Number(res.data.open ?? 0)
  summary.blocked = Number(res.data.blocked ?? 0)
}
async function loadRules() {
  loading.value = true
  try { rules.value = (await govApi.complianceRules()).data || [] } finally { loading.value = false }
}
async function loadViolations() {
  violations.value = (await govApi.complianceViolations({ status: vStatus.value, severity: vSeverity.value })).data || []
}
function openRule(row?: ComplianceRuleRow) {
  Object.assign(form, row ? {
    id: row.id, code: row.code, name: row.name, ruleType: row.rule_type, objectType: row.object_type,
    field: row.field, op: row.op, ruleValue: row.rule_value, conditionExpr: row.condition_expr,
    severity: row.severity, doc: row.doc, description: row.description,
  } : {
    id: 0, code: '', name: '', ruleType: 'required', objectType: 'contract', field: '',
    op: '', ruleValue: '', conditionExpr: '', severity: 'warn', doc: '', description: '',
  })
  ruleDialog.value = true
}
async function saveRule() {
  const d = {
    code: form.code, name: form.name, ruleType: form.ruleType, objectType: form.objectType,
    field: form.field, op: form.op, ruleValue: form.ruleValue, conditionExpr: form.conditionExpr,
    severity: form.severity, doc: form.doc, description: form.description,
  }
  if (form.id) await govApi.complianceRuleUpdate(form.id, d)
  else await govApi.complianceRuleSave(d)
  ElMessage.success('规则已保存')
  ruleDialog.value = false
  await Promise.all([loadRules(), loadSummary()])
}
async function toggleRule(row: ComplianceRuleRow) {
  await govApi.complianceRuleToggle(row.id)
  ElMessage.success(row.enabled ? '已停用' : '已启用')
  await Promise.all([loadRules(), loadSummary()])
}
async function showHistory(row: ComplianceRuleRow) {
  histories.value = (await govApi.complianceRuleHistory(row.id)).data || []
  historyDialog.value = true
}
function openHandle(row: ComplianceViolationRow) {
  current.value = row
  handleAction.value = 'resolved'
  handleNote.value = ''
  handleDialog.value = true
}
async function submitHandle() {
  if (!current.value) return
  await govApi.complianceViolationHandle(current.value.id, handleAction.value, handleNote.value)
  ElMessage.success('已处理')
  handleDialog.value = false
  await Promise.all([loadViolations(), loadSummary()])
}
async function runCheck() {
  let record: Record<string, unknown> = {}
  try { record = JSON.parse(checkJson.value) } catch { ElMessage.error('业务记录不是合法 JSON'); return }
  const res = await govApi.complianceCheck(checkType.value, record)
  checkResults.value = res.data?.results || []
  ElMessage.success(`校验 ${res.data?.evaluated || 0} 条，未通过 ${res.data?.failed || 0} 条`)
}
onMounted(loadAll)
</script>

<style scoped>
.tabs { margin-top: 12px }
.stats { margin-top: 12px }
.stat-label { font-size: 12px; color: #909399 }
.stat-value { font-size: 26px; font-weight: 600; margin-top: 4px }
.stat-value.warn { color: #e6a23c }
.stat-value.danger { color: #f56c6c }
.toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; flex-wrap: wrap }
.hint { font-size: 12px; color: #909399 }
</style>
