<template>
  <div>
    <el-alert type="info" :closable="false" show-icon
      title="MDM 实体解析与去重（产品② US-G6 / AG6.1–AG6.5）"
      description="按可配置字段与阈值扫描疑似重复主数据并给出相似度说明与合并建议；确认合并后保留唯一权威记录、写入稳定 ID 映射，并把历史关联关系重定向到权威记录，不丢关系。" />

    <el-row :gutter="12" class="stats">
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">活跃主数据</div><div class="stat-value">{{ summary.records }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">待处理疑似重复</div><div class="stat-value warn">{{ summary.pending }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">已合并</div><div class="stat-value">{{ summary.merged }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">稳定 ID 映射</div><div class="stat-value">{{ summary.mappings }}</div></el-card></el-col>
    </el-row>

    <el-tabs v-model="tab" class="tabs">
      <!-- AG6.5 匹配规则 -->
      <el-tab-pane label="匹配规则" name="rules">
        <div class="toolbar">
          <el-button type="primary" @click="openRule()">新增规则</el-button>
          <el-button @click="loadRules">刷新</el-button>
          <span class="hint">匹配字段与相似度阈值可配置；税号完全一致按强身份信号提升置信度。</span>
        </div>
        <el-table :data="rules" border stripe size="small">
          <el-table-column prop="id" label="ID" width="65" />
          <el-table-column prop="name" label="规则名称" min-width="170" />
          <el-table-column prop="md_type" label="主数据类型" width="120" />
          <el-table-column label="匹配字段" min-width="220">
            <template #default="{ row }"><el-tag v-for="f in parseFields(row.fields)" :key="f" size="small" style="margin-right: 4px">{{ f }}</el-tag></template>
          </el-table-column>
          <el-table-column label="阈值" width="90">
            <template #default="{ row }">{{ row.threshold }}</template>
          </el-table-column>
          <el-table-column label="状态" width="85">
            <template #default="{ row }"><el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openRule(row)">编辑</el-button>
              <el-button link type="success" @click="scan(row.id)">用此规则扫描</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- AG6.1/6.2 疑似重复 -->
      <el-tab-pane label="疑似重复" name="pairs">
        <div class="toolbar">
          <el-button type="primary" @click="scan(0)">扫描全部活跃主数据</el-button>
          <el-select v-model="pStatus" style="width: 150px" @change="loadPairs">
            <el-option label="全部状态" value="" />
            <el-option label="待处理" value="pending" />
            <el-option label="已合并" value="merged" />
            <el-option label="已判不同" value="ignored" />
          </el-select>
          <el-button @click="loadPairs">刷新</el-button>
          <span class="hint">只对同类型、状态 active 的记录两两比对。</span>
        </div>
        <el-table :data="pairs" border stripe size="small" v-loading="scanning">
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column label="候选 A" min-width="180">
            <template #default="{ row }">#{{ row.left_id }} {{ row.left_name }}<div class="hint">{{ row.left_uid }}</div></template>
          </el-table-column>
          <el-table-column label="候选 B" min-width="180">
            <template #default="{ row }">#{{ row.right_id }} {{ row.right_name }}<div class="hint">{{ row.right_uid }}</div></template>
          </el-table-column>
          <el-table-column label="相似度" width="100">
            <template #default="{ row }"><el-tag size="small" :type="row.score >= 90 ? 'danger' : 'warning'">{{ row.score }}</el-tag></template>
          </el-table-column>
          <el-table-column label="合并建议 / 判定理由" min-width="280">
            <template #default="{ row }">
              <div>{{ row.suggestion }}</div>
              <el-popover placement="top" width="420" trigger="click">
                <template #reference><el-button link type="info" size="small">查看逐字段说明</el-button></template>
                <div v-for="(r, i) in row.reasons || []" :key="i" class="hint">{{ r }}</div>
              </el-popover>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="95">
            <template #default="{ row }"><el-tag size="small" :type="row.status === 'pending' ? 'warning' : 'success'">{{ statusLabel(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <template v-if="row.status === 'pending'">
                <el-button link type="primary" @click="openMerge(row)">确认合并</el-button>
                <el-button link type="warning" @click="ignore(row)">判为不同</el-button>
              </template>
              <span v-else class="hint">{{ row.handled_note || '-' }}</span>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- AG6.3 权威版本与 ID 映射 -->
      <el-tab-pane label="ID 映射" name="mappings">
        <div class="toolbar"><el-button @click="loadMappings">刷新</el-button><span class="hint">被合并方的稳定 ID 永久可反查，下游引用不失效。</span></div>
        <el-table :data="mappings" border stripe size="small">
          <el-table-column prop="old_id" label="原记录 ID" width="100" />
          <el-table-column prop="old_uid" label="原稳定 ID" min-width="200" />
          <el-table-column prop="old_name" label="原名称" min-width="180" />
          <el-table-column prop="survivor_id" label="权威记录 ID" width="110" />
          <el-table-column prop="survivor_uid" label="权威稳定 ID" min-width="200" />
          <el-table-column label="合并时间" width="170">
            <template #default="{ row }">{{ fmt(row.merged_at) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="ruleDialog" :title="ruleForm.id ? '编辑匹配规则' : '新增匹配规则'" width="520px">
      <el-form :model="ruleForm" label-width="110px">
        <el-form-item label="规则名称"><el-input v-model="ruleForm.name" /></el-form-item>
        <el-form-item label="主数据类型"><el-input v-model="ruleForm.mdType" placeholder="supplier / customer ..." /></el-form-item>
        <el-form-item label="匹配字段">
          <el-select v-model="ruleForm.fields" multiple style="width: 100%">
            <el-option v-for="f in ['name', 'code', 'taxNo', 'address', 'phone', 'legalPerson']" :key="f" :label="f" :value="f" />
          </el-select>
        </el-form-item>
        <el-form-item label="相似度阈值"><el-input-number v-model="ruleForm.threshold" :min="1" :max="100" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="mergeDialog" title="确认合并" width="540px">
      <div class="hint" style="margin-bottom: 10px">{{ current?.suggestion }}</div>
      <el-radio-group v-model="survivorId">
        <el-radio :value="current?.left_id || 0">保留 A：{{ current?.left_name }}</el-radio>
        <el-radio :value="current?.right_id || 0">保留 B：{{ current?.right_name }}</el-radio>
      </el-radio-group>
      <el-input v-model="mergeNote" style="margin-top: 12px" placeholder="合并备注（可选）" />
      <template #footer>
        <el-button @click="mergeDialog = false">取消</el-button>
        <el-button type="primary" @click="submitMerge">确认合并</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { govApi, type MdmRuleRow, type MdmPairRow, type MdmMappingRow } from '../api'

const tab = ref('pairs')
const scanning = ref(false)
const rules = ref<MdmRuleRow[]>([])
const pairs = ref<MdmPairRow[]>([])
const mappings = ref<MdmMappingRow[]>([])
const summary = reactive({ records: 0, pending: 0, merged: 0, mappings: 0 })
const pStatus = ref('pending')
const ruleDialog = ref(false)
const ruleForm = reactive({ id: 0, name: '', mdType: 'supplier', fields: ['name', 'taxNo'], threshold: 70 })
const mergeDialog = ref(false)
const mergeNote = ref('')
const survivorId = ref(0)
const current = ref<MdmPairRow | null>(null)

function fmt(v?: string) { return v ? String(v).replace('T', ' ').slice(0, 19) : '-' }
function parseFields(s?: string): string[] { try { return JSON.parse(s || '[]') } catch { return [] } }
function statusLabel(s: string) { return ({ pending: '待处理', merged: '已合并', ignored: '已判不同' } as Record<string, string>)[s] || s }

async function loadAll() { await Promise.all([loadRules(), loadPairs(), loadMappings(), loadSummary()]) }
async function loadSummary() {
  const res = await govApi.mdmSummary()
  if (!res.data) return
  // 逐属性赋值，确保响应式追踪到每个字段
  summary.records = Number(res.data.records ?? 0)
  summary.pending = Number(res.data.pending ?? 0)
  summary.merged = Number(res.data.merged ?? 0)
  summary.mappings = Number(res.data.mappings ?? 0)
}
async function loadRules() { rules.value = (await govApi.mdmRules()).data || [] }
async function loadPairs() { pairs.value = (await govApi.mdmDuplicates(pStatus.value)).data || [] }
async function loadMappings() { mappings.value = (await govApi.mdmMappings()).data || [] }
function openRule(row?: MdmRuleRow) {
  Object.assign(ruleForm, row ? {
    id: row.id, name: row.name, mdType: row.md_type, fields: parseFields(row.fields), threshold: Number(row.threshold),
  } : { id: 0, name: '', mdType: 'supplier', fields: ['name', 'taxNo'], threshold: 70 })
  ruleDialog.value = true
}
async function saveRule() {
  await govApi.mdmRuleSave({
    id: ruleForm.id, name: ruleForm.name, mdType: ruleForm.mdType,
    fields: ruleForm.fields as unknown as string, threshold: ruleForm.threshold,
  })
  ElMessage.success('匹配规则已保存')
  ruleDialog.value = false
  await loadRules()
}
async function scan(ruleId: number) {
  scanning.value = true
  try {
    const res = await govApi.mdmScan(ruleId)
    const d = res.data
    ElMessage.success(`比对 ${d?.compared || 0} 对，命中 ${d?.pairs || 0} 对，新增 ${d?.created || 0} 条待办`)
    tab.value = 'pairs'
    await Promise.all([loadPairs(), loadSummary()])
  } finally { scanning.value = false }
}
function openMerge(row: MdmPairRow) {
  current.value = row
  survivorId.value = row.left_id
  mergeNote.value = ''
  mergeDialog.value = true
}
async function submitMerge() {
  if (!current.value) return
  const res = await govApi.mdmMerge(current.value.id, survivorId.value, mergeNote.value)
  ElMessage.success(`已合并，关联关系重定向 ${res.data?.linksRedirected ?? 0} 条`)
  mergeDialog.value = false
  await Promise.all([loadPairs(), loadMappings(), loadSummary()])
}
async function ignore(row: MdmPairRow) {
  const note = window.prompt('判为不同实体，请填写说明（必填，写入审计）：')
  if (note === null) return
  if (!note.trim()) { ElMessage.error('说明不能为空'); return }
  await govApi.mdmIgnore(row.id, note)
  ElMessage.success('已判为不同实体')
  await Promise.all([loadPairs(), loadSummary()])
}
onMounted(loadAll)
</script>

<style scoped>
.tabs { margin-top: 12px }
.stats { margin-top: 12px }
.stat-label { font-size: 12px; color: #909399 }
.stat-value { font-size: 26px; font-weight: 600; margin-top: 4px }
.stat-value.warn { color: #e6a23c }
.toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; flex-wrap: wrap }
.hint { font-size: 12px; color: #909399 }
</style>
