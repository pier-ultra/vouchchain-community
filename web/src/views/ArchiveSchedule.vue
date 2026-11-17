<template>
  <div>
    <el-alert type="info" :closable="false" show-icon
      title="定期自动归档调度（US-I9 / AI9.1–AI9.4，P1）"
      description="按账期（周 / 月 / 季）结束后第 N 天凌晨 02:00 自动执行，复用 AI5 归档动作；只归档上一账期及更早、且满足验真 / 已入账 / 已付款门槛的发票。失败项写异常队列并推消息中心，需人工处理。" />
    <el-tabs v-model="tab" class="tabs">
      <!-- AI9.1 调度配置 -->
      <el-tab-pane label="调度配置" name="config">
        <div class="toolbar">
          <el-button type="primary" @click="openDialog()">新增调度</el-button>
          <el-button @click="loadSchedules">刷新</el-button>
          <span class="hint">后台每 60 秒扫描到期配置，原子抢锁后执行（多实例不会重复归档）。</span>
        </div>
        <el-table :data="schedules" border stripe v-loading="loading">
          <el-table-column prop="id" label="ID" width="65" />
          <el-table-column prop="name" label="名称" min-width="150" />
          <el-table-column label="账期" width="90">
            <template #default="{ row }">{{ periodLabel(row.period_type) }}</template>
          </el-table-column>
          <el-table-column label="账期后" width="90">
            <template #default="{ row }">第 {{ row.due_days }} 天</template>
          </el-table-column>
          <el-table-column label="范围" width="130">
            <template #default="{ row }">
              {{ row.scope_type === 'legal_entity' ? `主体 #${row.scope_id}` : '全部主体' }}
            </template>
          </el-table-column>
          <el-table-column label="归档门槛" min-width="190">
            <template #default="{ row }">
              <el-tag size="small" :type="row.require_verified ? 'success' : 'info'">已验真</el-tag>
              <el-tag size="small" :type="row.require_voucher ? 'success' : 'info'" style="margin-left: 4px">已入账</el-tag>
              <el-tag size="small" :type="row.require_paid ? 'success' : 'info'" style="margin-left: 4px">已付款</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="下次执行" width="165">
            <template #default="{ row }">{{ fmt(row.next_run_at) }}</template>
          </el-table-column>
          <el-table-column label="上次执行" width="165">
            <template #default="{ row }">{{ fmt(row.last_run_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="280" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="runNow(row)">立即执行</el-button>
              <el-button link type="primary" @click="openDialog(row)">编辑</el-button>
              <el-button link type="warning" @click="toggle(row)">{{ row.enabled ? '停用' : '启用' }}</el-button>
              <el-button link type="danger" @click="remove(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- AI9.3 执行日志 -->
      <el-tab-pane label="执行日志" name="runs">
        <div class="toolbar">
          <el-button @click="loadRuns">刷新</el-button>
          <span class="hint">每次调度记录命中 / 成功 / 失败项，可展开查看逐票结果。</span>
        </div>
        <el-table :data="runs" border stripe>
          <el-table-column type="expand">
            <template #default="{ row }">
              <div class="expand-box">
                <el-button size="small" @click="loadItems(row.id)">加载本次逐票明细</el-button>
                <el-table v-if="itemsByRun[row.id]" :data="itemsByRun[row.id]" size="small" border style="margin-top: 8px">
                  <el-table-column prop="invoice_no" label="发票号" width="160" />
                  <el-table-column prop="invoice_id" label="发票ID" width="90" />
                  <el-table-column label="结果" width="90">
                    <template #default="{ r }">
                      <el-tag size="small" :type="r.result === 'ok' ? 'success' : r.result === 'failed' ? 'danger' : 'info'">
                        {{ r.result }}
                      </el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column prop="message" label="说明" />
                  <el-table-column prop="archive_path" label="归档包" min-width="180" />
                </el-table>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="id" label="运行ID" width="90" />
          <el-table-column prop="schedule_id" label="调度ID" width="90" />
          <el-table-column label="触发" width="90">
            <template #default="{ row }">{{ row.trigger === 'auto' ? '自动' : '手动' }}</template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'done' ? 'success' : 'danger'">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="命中/成功/失败" width="150">
            <template #default="{ row }">{{ row.total }} / {{ row.success }} / <b class="fail">{{ row.failed }}</b></template>
          </el-table-column>
          <el-table-column prop="cutoff_date" label="账期截止" width="120" />
          <el-table-column prop="message" label="结果说明" min-width="220" />
          <el-table-column label="开始" width="165">
            <template #default="{ row }">{{ fmt(row.started_at) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- AI9.4 异常告警与人工处理 -->
      <el-tab-pane label="异常待办" name="exceptions">
        <div class="toolbar">
          <el-select v-model="exceptionStatus" style="width: 150px" @change="loadExceptions">
            <el-option label="待处理" value="pending" />
            <el-option label="已解决" value="resolved" />
            <el-option label="已忽略" value="ignored" />
            <el-option label="全部" value="" />
          </el-select>
          <el-button @click="loadExceptions">刷新</el-button>
          <el-badge :value="pendingCount" :hidden="pendingCount === 0" />
          <span class="hint">归档失败项同时推送消息中心；补齐四性检测 / 源件后可重试归档，或登记说明忽略。</span>
        </div>
        <el-table :data="exceptions" border stripe>
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column prop="invoice_no" label="发票号" width="160" />
          <el-table-column prop="invoice_id" label="发票ID" width="90" />
          <el-table-column prop="reason" label="失败原因" min-width="220" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'pending' ? 'danger' : row.status === 'resolved' ? 'success' : 'info'">
                {{ { pending: '待处理', resolved: '已解决', ignored: '已忽略' }[row.status] || row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="handled_by" label="处理人" width="110" />
          <el-table-column prop="handled_note" label="处理说明" min-width="180" />
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <template v-if="row.status === 'pending'">
                <el-button link type="primary" @click="handle(row, 'retry')">重试归档</el-button>
                <el-button link type="warning" @click="handle(row, 'ignore')">忽略</el-button>
              </template>
              <span v-else class="hint">已处理</span>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 调度配置编辑 -->
    <el-dialog v-model="dialog" :title="form.id ? '编辑调度' : '新增调度'" width="560px">
      <el-form label-width="120px">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如：月度进项票归档" /></el-form-item>
        <el-form-item label="账期">
          <el-select v-model="form.periodType" style="width: 100%">
            <el-option label="按周" value="weekly" />
            <el-option label="按月" value="monthly" />
            <el-option label="按季" value="quarterly" />
          </el-select>
        </el-form-item>
        <el-form-item label="账期结束后">
          <el-input-number v-model="form.dueDays" :min="1" :max="28" />
          <span class="form-hint">第 N 天凌晨 02:00 执行</span>
        </el-form-item>
        <el-form-item label="归档范围">
          <el-select v-model="form.scopeType" style="width: 100%">
            <el-option label="全部主体" value="all" />
            <el-option label="指定法人主体" value="legal_entity" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.scopeType === 'legal_entity'" label="指定主体">
          <el-select v-model="form.scopeId" style="width: 100%">
            <el-option v-for="e in entities" :key="e.id" :label="e.name" :value="e.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="归档门槛">
          <el-checkbox v-model="form.requireVerified">已验真</el-checkbox>
          <el-checkbox v-model="form.requireVoucher">已关联凭证</el-checkbox>
          <el-checkbox v-model="form.requirePaid">已付款</el-checkbox>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import {
  api, type ArchiveScheduleRow, type ArchiveScheduleRunRow, type ArchiveScheduleItemRow,
  type ArchiveExceptionRow, type LegalEntityRow,
} from '../api'

const tab = ref('config')
const loading = ref(false)
const schedules = ref<ArchiveScheduleRow[]>([])
const runs = ref<ArchiveScheduleRunRow[]>([])
const exceptions = ref<ArchiveExceptionRow[]>([])
const entities = ref<LegalEntityRow[]>([])
const exceptionStatus = ref('pending')
const itemsByRun = reactive<Record<number, ArchiveScheduleItemRow[]>>({})
const pendingCount = ref(0)

function fmt(t?: string) {
  return t ? String(t).replace('T', ' ').slice(0, 19) : '—'
}
function periodLabel(p: string) {
  return { weekly: '按周', monthly: '按月', quarterly: '按季' }[p] || p
}
async function loadSchedules() {
  loading.value = true
  try {
    const res = await api.archiveSchedules()
    schedules.value = res.data || []
  } finally {
    loading.value = false
  }
}
async function loadRuns() {
  const res = await api.archiveScheduleRuns()
  runs.value = res.data || []
}
async function loadItems(runId: number) {
  const res = await api.archiveScheduleRunItems(runId)
  itemsByRun[runId] = res.data || []
}
async function loadExceptions() {
  const res = await api.archiveExceptions(exceptionStatus.value || undefined)
  exceptions.value = res.data || []
  const all = await api.archiveExceptions('pending')
  pendingCount.value = (all.data || []).length
}

// ---------- AI9.1 配置 ----------
const dialog = ref(false)
const form = reactive({
  id: 0, name: '', periodType: 'monthly', dueDays: 5, scopeType: 'all', scopeId: 0,
  requireVerified: true, requireVoucher: true, requirePaid: false, enabled: true,
})
function openDialog(row?: ArchiveScheduleRow) {
  Object.assign(form, row
    ? { id: row.id, name: row.name, periodType: row.period_type, dueDays: row.due_days,
        scopeType: row.scope_type, scopeId: row.scope_id, requireVerified: row.require_verified,
        requireVoucher: row.require_voucher, requirePaid: row.require_paid, enabled: row.enabled }
    : { id: 0, name: '', periodType: 'monthly', dueDays: 5, scopeType: 'all', scopeId: 0,
        requireVerified: true, requireVoucher: true, requirePaid: false, enabled: true })
  dialog.value = true
}
function payload() {
  return {
    name: form.name, periodType: form.periodType, dueDays: form.dueDays,
    scopeType: form.scopeType, scopeId: form.scopeId,
    requireVerified: form.requireVerified, requireVoucher: form.requireVoucher,
    requirePaid: form.requirePaid, enabled: form.enabled,
  }
}
async function save() {
  if (!form.name.trim()) return ElMessage.warning('调度名称必填')
  if (form.scopeType === 'legal_entity' && !form.scopeId) return ElMessage.warning('按主体归档请选择具体主体')
  if (form.id) await api.archiveScheduleUpdate(form.id, payload())
  else await api.archiveScheduleCreate(payload())
  dialog.value = false
  ElMessage.success('已保存')
  await loadSchedules()
}
async function runNow(row: ArchiveScheduleRow) {
  await ElMessageBox.confirm(`立即执行「${row.name}」？将按当前账期截止日归档满足门槛的发票。`, '提示', { type: 'warning' })
  const res = await api.archiveScheduleRun(row.id)
  ElMessage.success(`执行完成，运行 #${res.data?.runId}，请到「执行日志」查看结果`)
  await Promise.all([loadRuns(), loadExceptions(), loadSchedules()])
  tab.value = 'runs'
}
async function toggle(row: ArchiveScheduleRow) {
  await api.archiveScheduleToggle(row.id)
  await loadSchedules()
}
async function remove(row: ArchiveScheduleRow) {
  await ElMessageBox.confirm(`确认删除调度「${row.name}」？历史执行日志保留。`, '提示', { type: 'warning' })
  await api.archiveScheduleDelete(row.id)
  await loadSchedules()
}

// ---------- AI9.4 人工处理 ----------
async function handle(row: ArchiveExceptionRow, action: 'retry' | 'ignore') {
  let note = ''
  if (action === 'ignore') {
    const input = await ElMessageBox.prompt('忽略原因（必填，写入审计）', '忽略归档异常', {
      inputPlaceholder: '如：该票已作废，无需归档',
    })
    note = input.value || ''
    if (!note.trim()) return ElMessage.warning('忽略必须填写说明')
  } else {
    await ElMessageBox.confirm('确认已补齐缺件（四性检测 / 源件），现在重试归档？', '提示', { type: 'warning' })
  }
  await api.archiveExceptionHandle(row.id, action, note)
  ElMessage.success(action === 'retry' ? '重试归档成功，异常已解决' : '已登记忽略')
  await loadExceptions()
}

onMounted(async () => {
  const e = await api.legalEntities()
  entities.value = e.data?.list || []
  await Promise.all([loadSchedules(), loadRuns(), loadExceptions()])
})
</script>

<style scoped>
.tabs { padding: 0 4px; }
.toolbar { display: flex; align-items: center; gap: 10px; margin: 12px 0; flex-wrap: wrap; }
.hint { color: #909399; font-size: 12px; }
.form-hint { color: #909399; font-size: 12px; margin-left: 10px; }
.fail { color: #f56c6c; }
.expand-box { padding: 8px 18px; }
</style>
