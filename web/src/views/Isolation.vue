<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">强隔离策略</span>
        <span class="card-sub">AG5.1–AG5.5：法人 / 部门 / 项目三维隔离，策略压过主体切换，越权即拦截并留痕</span>
      </div>
      <el-button v-if="can('isolation:write')" type="primary" @click="openDialog()">
        <el-icon><Plus /></el-icon>新增策略
      </el-button>
    </div>

    <el-row :gutter="12" class="cards">
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">策略总数</div><div class="stat-value">{{ summary.total }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">已启用</div><div class="stat-value ok">{{ summary.enabled }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">越权拦截记录</div><div class="stat-value">{{ summary.violations }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">待处理告警</div><div class="stat-value warn">{{ summary.open }}</div></el-card></el-col>
    </el-row>

    <el-tabs v-model="tab">
      <el-tab-pane label="隔离策略" name="policies">
        <el-alert type="info" :closable="false" show-icon class="hint">
          用户级策略完全覆盖角色级 / 全局级；同级多条取并集。策略启用后即为强制条件，
          即便请求头切换主体或选「全部主体」也只能看到策略允许范围。
        </el-alert>
        <el-table :data="policies" border stripe v-loading="loading">
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column prop="name" label="策略名" min-width="190" />
          <el-table-column label="维度" width="100">
            <template #default="{ row }">{{ dimLabel(row.dimension) }}</template>
          </el-table-column>
          <el-table-column label="授权对象" width="150">
            <template #default="{ row }">
              {{ row.subjectType === 'all' ? '全部用户' : (row.subjectType === 'user' ? '用户 · ' : '角色 · ') + row.subject }}
            </template>
          </el-table-column>
          <el-table-column label="允许范围" min-width="180">
            <template #default="{ row }">
              <el-tag v-for="v in row.allowedValues" :key="v" size="small" class="val-tag">{{ v }}</el-tag>
              <span v-if="!row.allowedValues?.length" class="muted">空（拒绝全部）</span>
              <el-tag v-if="row.includeChildren" size="small" type="success" class="val-tag">含下级</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column v-if="can('isolation:write')" label="操作" width="200" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="toggle(row)">{{ row.enabled ? '停用' : '启用' }}</el-button>
              <el-button link type="primary" @click="openDialog(row)">编辑</el-button>
              <el-button link type="danger" @click="remove(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="当前生效范围" name="effective">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="当前用户">{{ effective.username || '—' }}</el-descriptions-item>
          <el-descriptions-item label="生效角色">{{ effective.role || '—' }}</el-descriptions-item>
          <el-descriptions-item label="强制隔离">
            <el-tag :type="effective.enforced ? 'danger' : 'success'">
              {{ effective.enforced ? '已启用（强制 AND 条件）' : '未启用（沿用原有数据权限）' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="命中策略">{{ (effective.matched || []).join(', ') || '—' }}</el-descriptions-item>
        </el-descriptions>
        <div class="section-title">各维度允许范围</div>
        <el-table :data="dimensionRows" border stripe>
          <el-table-column prop="label" label="维度" width="130" />
          <el-table-column label="允许值" min-width="260">
            <template #default="{ row }">
              <el-tag v-for="v in row.values" :key="v" size="small" class="val-tag">{{ v }}</el-tag>
              <span v-if="!row.configured" class="muted">未配置（不限制）</span>
              <span v-else-if="!row.values.length" class="muted">空（拒绝全部）</span>
            </template>
          </el-table-column>
          <el-table-column prop="source" label="来源策略" min-width="220" />
          <el-table-column label="级别覆盖" width="110">
            <template #default="{ row }">
              <el-tag v-if="row.overridden" size="small" type="warning">用户/角色级</el-tag>
              <span v-else class="muted">—</span>
            </template>
          </el-table-column>
        </el-table>
        <el-alert type="warning" :closable="false" show-icon class="hint">
          历史未归属数据在法人 / 部门维度记为 0、项目维度为空串；强隔离启用后默认不可见，
          需在允许值里显式写入 <b>0</b> 或 <b>*</b> 才会放行。
        </el-alert>
      </el-tab-pane>

      <el-tab-pane label="越权拦截" name="violations">
        <div class="filters">
          <el-radio-group v-model="violationStatus" @change="loadViolations">
            <el-radio-button value="">全部</el-radio-button>
            <el-radio-button value="open">待处理</el-radio-button>
            <el-radio-button value="resolved">已处理</el-radio-button>
          </el-radio-group>
        </div>
        <el-table :data="violations" border stripe>
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column label="时间" width="160">
            <template #default="{ row }">{{ fmt(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="username" label="用户" width="110" />
          <el-table-column prop="path" label="请求" min-width="210" />
          <el-table-column prop="attempted_value" label="对象归属" min-width="220" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'open' ? 'danger' : 'success'">{{ row.status === 'open' ? '待处理' : row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column v-if="can('isolation:write')" label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <el-button v-if="row.status === 'open'" link type="primary" @click="handle(row)">标记处理</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="口径说明" name="guide">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="AG5.1 维度">法人主体（legal_entity_id）、组织部门（dept_id）、项目（project_code），可叠加，同时满足才放行。</el-descriptions-item>
          <el-descriptions-item label="AG5.2 可见范围">按用户 / 角色 / 全体配置；用户级覆盖角色级，角色级覆盖全局级；同级多条取并集。</el-descriptions-item>
          <el-descriptions-item label="AG5.3 隔离执行">业务列表自动注入强制 AND 条件；按 ID 的单条读写由对象级守卫逐一校验归属。</el-descriptions-item>
          <el-descriptions-item label="AG5.4 越权拦截">越权返回 403，写越权记录、审计轨迹与站内告警，可在本页结案。</el-descriptions-item>
          <el-descriptions-item label="AG5.5 变更审计">策略新建 / 修改 / 启停 / 删除均写入审计轨迹（含完整快照）。</el-descriptions-item>
          <el-descriptions-item label="诚实边界">这是应用内的数据可见范围控制，不是数据库行级安全（RLS）或独立租户实例；部署侧仍需管好数据库账号。</el-descriptions-item>
        </el-descriptions>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑隔离策略' : '新增隔离策略'" width="560px">
      <el-form label-width="100px">
        <el-form-item label="策略名" required><el-input v-model="form.name" placeholder="如：业务部仅华东主体" /></el-form-item>
        <el-form-item label="隔离维度" required>
          <el-select v-model="form.dimension" style="width: 100%">
            <el-option v-for="d in dimensions" :key="d.value" :label="d.label" :value="d.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="授权对象" required>
          <div class="inline">
            <el-select v-model="form.subjectType" style="width: 120px">
              <el-option label="按角色" value="role" />
              <el-option label="按用户" value="user" />
              <el-option label="全体" value="all" />
            </el-select>
            <el-input v-if="form.subjectType !== 'all'" v-model="form.subject" placeholder="角色码 / 用户名" style="flex: 1" />
            <span v-else class="muted">对所有已登录用户生效</span>
          </div>
        </el-form-item>
        <el-form-item label="允许值" required>
          <el-input v-model="form.valuesStr" placeholder="逗号分隔，如 1,2,3 或 PRJ-A,PRJ-B" />
          <div class="muted">{{ dimensions.find((d) => d.value === form.dimension)?.valueLabel }}</div>
        </el-form-item>
        <el-form-item v-if="form.dimension !== 'project'" label="含下级">
          <el-switch v-model="form.includeChildren" />
          <span class="muted">沿父子组织树递归展开</span>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="优先级"><el-input-number v-model="form.priority" :min="0" :max="999" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { govApi, type IsoDimensionRow, type IsoEffectiveRow, type IsoPolicyRow, type IsoViolationRow } from '../api'
import { can } from '../rbac'

const tab = ref('policies')
const loading = ref(false)
const saving = ref(false)
const policies = ref<IsoPolicyRow[]>([])
const dimensions = ref<IsoDimensionRow[]>([])
const violations = ref<IsoViolationRow[]>([])
const violationStatus = ref('')
const summary = reactive({ total: 0, enabled: 0, violations: 0, open: 0 })
const effective = ref<Partial<IsoEffectiveRow>>({})
const dialogVisible = ref(false)
const editingId = ref(0)
const form = reactive({
  name: '', dimension: 'entity', subjectType: 'role', subject: '',
  valuesStr: '', includeChildren: false, enabled: true, priority: 0,
})

const dimensionRows = computed(() => [
  { key: 'entity', label: '法人主体' },
  { key: 'dept', label: '组织部门' },
  { key: 'project', label: '项目' },
].map((d) => ({
  label: d.label,
  configured: Object.prototype.hasOwnProperty.call(effective.value.allowed || {}, d.key),
  values: effective.value.allowed?.[d.key] || [],
  source: effective.value.sourceLabels?.[d.key] || '—',
  overridden: (effective.value.overrides || []).includes(d.key),
})))

function dimLabel(v: string) {
  return dimensions.value.find((d) => d.value === v)?.label || v
}
function fmt(t?: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

async function loadAll() {
  loading.value = true
  try {
    const [d, p, e, s, v] = await Promise.all([
      govApi.isoDimensions(), govApi.isoPolicies(), govApi.isoEffective(),
      govApi.isoSummary(), govApi.isoViolations(violationStatus.value || undefined),
    ])
    dimensions.value = d.data || []
    policies.value = p.data || []
    effective.value = e.data || {}
    violations.value = v.data || []
    Object.assign(summary, s.data || {})
  } finally {
    loading.value = false
  }
}
async function loadViolations() {
  const v = await govApi.isoViolations(violationStatus.value || undefined)
  violations.value = v.data || []
}

function openDialog(row?: IsoPolicyRow) {
  editingId.value = row?.id || 0
  form.name = row?.name || ''
  form.dimension = row?.dimension || 'entity'
  form.subjectType = row?.subjectType || 'role'
  form.subject = row?.subject || ''
  form.valuesStr = (row?.allowedValues || []).join(',')
  form.includeChildren = row?.includeChildren || false
  form.enabled = row ? row.enabled : true
  form.priority = row?.priority || 0
  dialogVisible.value = true
}
async function save() {
  if (!form.name.trim() || !form.valuesStr.trim()) {
    ElMessage.warning('请填写策略名与允许值')
    return
  }
  saving.value = true
  try {
    await govApi.isoPolicySave({
      name: form.name.trim(), dimension: form.dimension,
      subjectType: form.subjectType, subject: form.subject.trim(),
      allowedValues: form.valuesStr.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
      includeChildren: form.includeChildren, enabled: form.enabled, priority: form.priority,
    }, editingId.value || undefined)
    ElMessage.success('策略已保存并即时生效')
    dialogVisible.value = false
    await loadAll()
  } finally {
    saving.value = false
  }
}
async function toggle(row: IsoPolicyRow) {
  await govApi.isoPolicyToggle(row.id)
  ElMessage.success(row.enabled ? '已停用' : '已启用')
  await loadAll()
}
async function remove(row: IsoPolicyRow) {
  try {
    await ElMessageBox.confirm(`删除策略「${row.name}」？删除后该维度限制立即解除。`, '确认', { type: 'warning' })
  } catch { return }
  await govApi.isoPolicyDelete(row.id)
  ElMessage.success('已删除')
  await loadAll()
}
async function handle(row: IsoViolationRow) {
  await govApi.isoViolationHandle(row.id, 'resolved', '管理员已核实')
  ElMessage.success('告警已结案')
  await loadAll()
}

onMounted(loadAll)
</script>

<style scoped>
.toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.card-title { font-size: 16px; font-weight: 600; margin-right: 12px; }
.card-sub { font-size: 12px; color: #909399; }
.cards { margin-bottom: 12px; }
.stat-label { font-size: 12px; color: #909399; }
.stat-value { font-size: 24px; font-weight: 600; margin-top: 4px; }
.stat-value.ok { color: #67c23a; }
.stat-value.warn { color: #e6a23c; }
.hint { margin-bottom: 12px; }
.section-title { font-weight: 600; margin: 16px 0 8px; }
.val-tag { margin-right: 4px; }
.muted { color: #909399; font-size: 12px; }
.inline { display: flex; gap: 8px; align-items: center; width: 100%; }
.filters { margin-bottom: 12px; }
</style>
