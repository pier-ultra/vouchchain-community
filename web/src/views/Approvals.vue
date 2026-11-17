<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">审批中心</span>
        <span class="card-sub">AG3.1–AG3.6：条件分支 / 会签或签 / 加签 / 转交 / 废止，节点流转全程留痕</span>
      </div>
      <el-button type="primary" @click="openTplDialog">
        <el-icon><Plus /></el-icon>
        新建模板
      </el-button>
    </div>

    <el-tabs v-model="tab">
      <!-- 审批模板（AC6.3） -->
      <el-tab-pane label="审批模板" name="tpl">
        <el-table :data="templates" border stripe>
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column prop="name" label="模板名" min-width="160" />
          <el-table-column label="流程节点" min-width="380">
            <template #default="{ row }">
              <span v-for="(n, i) in parseNodes(row.nodes)" :key="i" class="node-chip">
                {{ i + 1 }}. {{ n.name }}
                <el-tag size="small" :type="n.mode === 'and' ? 'warning' : 'info'">
                  {{ n.mode === 'and' ? '会签' : '或签' }}·{{ (n.approvers || []).join('/') }}
                </el-tag>
                <el-tag v-if="n.when && n.when.field" size="small" type="danger" class="cond-tag">
                  仅当 {{ n.when.field }} {{ n.when.op }} {{ n.when.value }}
                </el-tag>
              </span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="110">
            <template #default="{ row }">
              <el-button link type="primary" @click="startWith(row)">用于发起审批</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 审批实例（AC6.4） -->
      <el-tab-pane label="审批实例" name="inst">
        <div class="filters">
          <el-radio-group v-model="instStatus" @change="loadInstances">
            <el-radio-button value="">全部</el-radio-button>
            <el-radio-button value="running">进行中</el-radio-button>
            <el-radio-button value="approved">已通过</el-radio-button>
            <el-radio-button value="rejected">已驳回</el-radio-button>
            <el-radio-button value="revoked">已废止</el-radio-button>
          </el-radio-group>
          <el-input v-model="myApprover" placeholder="我的待办（审批人名）" clearable style="width: 200px" @keyup.enter="loadInstances" @clear="loadInstances" />
        </div>
        <el-table :data="instances" border stripe>
          <el-table-column prop="id" label="实例ID" width="80" />
          <el-table-column label="合同" width="110">
            <template #default="{ row }">#{{ row.target_id }}</template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag v-if="row.status === 'approved'" type="success">已通过</el-tag>
              <el-tag v-else-if="row.status === 'rejected'" type="danger">已驳回</el-tag>
              <el-tag v-else-if="row.status === 'revoked'" type="info">已废止</el-tag>
              <el-tag v-else type="warning">进行中(第{{ row.current_seq }}级)</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_by" label="发起人" width="110" />
          <el-table-column label="发起时间" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="showDetail(row)">详情/审批</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 模板编辑（节点动态增删，会签/或签） -->
    <el-dialog v-model="tplVisible" title="新建审批模板" width="640px">
      <el-form label-width="80px">
        <el-form-item label="模板名" required>
          <el-input v-model="tplForm.name" placeholder="如：标准合同三级审批" />
        </el-form-item>
      </el-form>
      <div v-for="(n, i) in tplForm.nodes" :key="i" class="node-block">
        <div class="node-row">
          <el-input v-model="n.name" placeholder="节点名（如：商务部审批）" style="width: 200px" />
          <el-input v-model="n.approversStr" placeholder="审批人，逗号分隔（如：李四,王五）" style="flex: 1" />
          <el-radio-group v-model="n.mode" size="small">
            <el-radio-button value="or">或签</el-radio-button>
            <el-radio-button value="and">会签</el-radio-button>
          </el-radio-group>
          <el-button link type="danger" @click="tplForm.nodes.splice(i, 1)">删除</el-button>
        </div>
        <div class="cond-row">
          <el-checkbox v-model="n.hasWhen" size="small">条件节点</el-checkbox>
          <template v-if="n.hasWhen">
            <span class="cond-text">仅当</span>
            <el-select v-model="n.whenField" size="small" style="width: 130px">
              <el-option v-for="f in contextFields" :key="f" :label="f" :value="f" />
            </el-select>
            <el-select v-model="n.whenOp" size="small" style="width: 90px">
              <el-option v-for="o in ['gt', 'gte', 'lt', 'lte', 'eq', 'ne', 'contains', 'in']" :key="o" :label="o" :value="o" />
            </el-select>
            <el-input v-model="n.whenValue" size="small" placeholder="阈值 / 值" style="width: 140px" />
          </template>
          <span v-else class="muted">无条件，始终参与审批</span>
        </div>
      </div>
      <el-button size="small" @click="addNode">+ 添加节点</el-button>
      <div class="muted">同一流程里配两个条件互斥的节点即可实现分支：金额小走主管、金额大走总监，不命中的整级自动跳过并留痕。</div>
      <template #footer>
        <el-button @click="tplVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveTemplate">保存</el-button>
      </template>
    </el-dialog>

    <!-- 发起审批 -->
    <el-dialog v-model="startVisible" title="发起审批" width="440px">
      <el-form label-width="90px">
        <el-form-item label="合同">
          <el-input :model-value="`#${startTarget?.id} ${startTarget?.filename || ''}`" disabled />
        </el-form-item>
        <el-form-item label="审批模板" required>
          <el-select v-model="startTemplateId" style="width: 100%">
            <el-option v-for="t in templates" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="startVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="confirmStart">发起</el-button>
      </template>
    </el-dialog>

    <!-- 实例详情：节点时间线 + 审批动作 -->
    <el-drawer v-model="detailVisible" :title="`审批实例 #${detailId}`" size="52%">
      <el-timeline>
        <el-timeline-item
          v-for="(n, i) in detailNodes"
          :key="i"
          :type="nodeType(n)"
          :timestamp="`第 ${n.seq} 级 · ${n.mode === 'and' ? '会签' : '或签'} · ${n.approver}`"
        >
          <b>{{ n.name }}</b>
          <el-tag size="small" style="margin-left: 6px" :type="nodeType(n)">{{ nodeStatusLabel(n.status) }}</el-tag>
          <el-tag v-if="n.origin && n.origin !== 'template'" size="small" type="warning" style="margin-left: 4px">
            {{ n.origin === 'addsign' ? '加签' : '转交' }}
          </el-tag>
          <el-tag v-if="n.when_expr" size="small" type="danger" style="margin-left: 4px">有条件</el-tag>
          <div v-if="n.comment" class="comment">{{ n.comment }}</div>
        </el-timeline-item>
      </el-timeline>

      <div v-if="detailInst?.status === 'running'" class="act-area">
        <el-divider>审批动作</el-divider>
        <el-form label-width="90px">
          <el-form-item label="审批身份">
            <el-select v-model="actForm.actor" style="width: 100%">
              <el-option v-for="a in currentApprovers" :key="a" :label="a" :value="a" />
            </el-select>
          </el-form-item>
          <el-form-item label="意见">
            <el-input v-model="actForm.comment" type="textarea" :rows="2" placeholder="通过可选填；驳回必填" />
          </el-form-item>
        </el-form>
        <el-button type="success" :loading="acting" @click="doAct('approve')">同意</el-button>
        <el-button type="danger" :loading="acting" @click="doAct('reject')">驳回</el-button>

        <el-divider>流程干预（AG3.4）</el-divider>
        <div class="flow-row">
          <el-input v-model="flow.addTo" placeholder="被加签人" style="width: 130px" />
          <el-select v-model="flow.addMode" style="width: 100px">
            <el-option label="会签" value="and" />
            <el-option label="或签" value="or" />
          </el-select>
          <el-input v-model="flow.addComment" placeholder="加签说明" style="flex: 1" />
          <el-button :loading="acting" @click="doAddSign">加签</el-button>
        </div>
        <div class="flow-row">
          <el-input v-model="flow.transferTo" placeholder="转交给谁" style="width: 130px" />
          <el-input v-model="flow.transferComment" placeholder="转交原因" style="flex: 1" />
          <el-button :loading="acting" @click="doTransfer">转交</el-button>
        </div>
        <div class="flow-row">
          <el-input v-model="flow.cancelReason" placeholder="废止原因（必填）" style="flex: 1" />
          <el-button type="danger" plain :loading="acting" @click="doCancel">废止流程</el-button>
        </div>
      </div>

      <el-divider>流转留痕（AG3.6）</el-divider>
      <el-table :data="events" border stripe size="small" max-height="240">
        <el-table-column label="时间" width="150">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="actor" label="操作人" width="100" />
        <el-table-column label="动作" width="100">
          <template #default="{ row }">{{ eventLabel(row.action) }}</template>
        </el-table-column>
        <el-table-column label="说明" min-width="200">
          <template #default="{ row }">
            {{ row.comment || '—' }}
            <span v-if="row.to_user" class="muted"> → {{ row.to_user }}</span>
            <span v-if="row.seq" class="muted">（第 {{ row.seq }} 级）</span>
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api, type ApprovalEventRow, type ApprovalNodeRow, type ApprovalRow, type ContractFile, type TemplateRow } from '../api'

interface NodeForm {
  name: string
  approversStr: string
  mode: string
  hasWhen: boolean
  whenField: string
  whenOp: string
  whenValue: string
}
const newNode = (): NodeForm => ({
  name: '', approversStr: '', mode: 'or', hasWhen: false,
  whenField: 'amount', whenOp: 'gte', whenValue: '',
})
const contextFields = ['amount', 'counterParty', 'status', 'riskLevel', 'projectCode', 'legalEntityId', 'targetType']

const tab = ref('tpl')
const templates = ref<TemplateRow[]>([])
const instances = ref<ApprovalRow[]>([])
const instStatus = ref('')
const myApprover = ref('')
const saving = ref(false)
const acting = ref(false)
const events = ref<ApprovalEventRow[]>([])

const tplVisible = ref(false)
const tplForm = reactive({ name: '', nodes: [newNode()] as NodeForm[] })

const startVisible = ref(false)
const startTarget = ref<ContractFile | null>(null)
const startTemplateId = ref<number | undefined>()

const detailVisible = ref(false)
const detailId = ref(0)
const detailInst = ref<ApprovalRow | null>(null)
const detailNodes = ref<ApprovalNodeRow[]>([])
const actForm = reactive({ actor: '', comment: '' })
const flow = reactive({ addTo: '', addMode: 'and', addComment: '', transferTo: '', transferComment: '', cancelReason: '' })

const currentSeq = computed(() => detailInst.value?.current_seq || 0)
const currentApprovers = computed(() =>
  detailNodes.value.filter((n) => n.seq === currentSeq.value && n.status === 'pending').map((n) => n.approver),
)

function parseNodes(raw: string) {
  try {
    return JSON.parse(raw || '[]')
  } catch {
    return []
  }
}

async function loadTemplates() {
  const res = await api.templateList()
  templates.value = res.data || []
}
async function loadInstances() {
  const res = await api.approvalList({
    status: instStatus.value || undefined,
    approver: myApprover.value || undefined,
  })
  instances.value = res.data || []
}

function addNode() {
  tplForm.nodes.push(newNode())
}
function nodeType(n: ApprovalNodeRow) {
  if (n.status === 'approved') return 'success'
  if (n.status === 'rejected') return 'danger'
  if (n.status === 'skipped' || n.status === 'revoked') return 'info'
  return n.seq === currentSeq.value ? 'warning' : 'info'
}
function nodeStatusLabel(s: string) {
  return { approved: '已通过', rejected: '已驳回', pending: '待审批', skipped: '条件跳过', revoked: '已废止' }[s] || s
}
function eventLabel(a: string) {
  return { create: '发起', approve: '同意', reject: '驳回', skip: '条件跳过', transfer: '转交', addsign: '加签', revoke: '废止' }[a] || a
}

function openTplDialog() {
  tplForm.name = ''
  tplForm.nodes = [newNode()]
  tplVisible.value = true
}

async function saveTemplate() {
  const nodes = tplForm.nodes
    .filter((n) => n.name.trim() && n.approversStr.trim())
    .map((n) => ({
      name: n.name.trim(),
      approvers: n.approversStr.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
      mode: n.mode,
      when: n.hasWhen && n.whenValue.trim()
        ? { field: n.whenField, op: n.whenOp, value: n.whenValue.trim() }
        : undefined,
    }))
  if (!tplForm.name.trim() || !nodes.length) {
    ElMessage.warning('请填写模板名与至少一个完整节点')
    return
  }
  saving.value = true
  try {
    await api.templateCreate(tplForm.name.trim(), nodes)
    ElMessage.success('模板已创建')
    tplVisible.value = false
    await loadTemplates()
  } finally {
    saving.value = false
  }
}

function startWith(tpl: TemplateRow) {
  startTemplateId.value = tpl.id
  startVisible.value = true
  if (!startTarget.value) {
    ElMessage.info('请到「任务中心」对合同点「发起审批」后再选模板')
  }
}

async function confirmStart() {
  if (!startTarget.value || !startTemplateId.value) return
  saving.value = true
  try {
    await api.approvalCreate(startTarget.value.id, startTemplateId.value)
    ElMessage.success('审批已发起，首节点审批人已收到通知')
    startVisible.value = false
    await Promise.all([loadInstances(), loadTemplates()])
  } finally {
    saving.value = false
  }
}

async function showDetail(row: ApprovalRow) {
  detailId.value = row.id
  detailVisible.value = true
  const [res, ev] = await Promise.all([api.approvalDetail(row.id), api.approvalEvents(row.id)])
  detailInst.value = res.data?.instance || null
  detailNodes.value = res.data?.nodes || []
  events.value = ev.data || []
  actForm.actor = currentApprovers.value[0] || ''
  actForm.comment = ''
}

async function doAct(action: 'approve' | 'reject') {
  if (!actForm.actor) {
    ElMessage.warning('请选择审批身份')
    return
  }
  if (action === 'reject' && !actForm.comment.trim()) {
    ElMessage.warning('驳回必须填写意见')
    return
  }
  acting.value = true
  try {
    const res = await api.approvalAct(detailId.value, actForm.actor, action, actForm.comment.trim())
    ElMessage.success(res.data?.status === 'approved' ? '审批全部通过' : res.data?.status === 'rejected' ? '已驳回，流程终止' : '已提交')
    await showDetail({ ...({ id: detailId.value } as ApprovalRow) })
    await loadInstances()
  } finally {
    acting.value = false
  }
}

async function doAddSign() {
  if (!actForm.actor || !flow.addTo.trim()) {
    ElMessage.warning('请选择审批身份并填写被加签人')
    return
  }
  acting.value = true
  try {
    await api.approvalAddSign(detailId.value, actForm.actor, flow.addTo.trim(), flow.addMode, flow.addComment.trim())
    ElMessage.success('已加签，该节点按' + (flow.addMode === 'and' ? '会签' : '或签') + '执行')
    flow.addTo = ''
    flow.addComment = ''
    await refreshDetail()
  } finally {
    acting.value = false
  }
}
async function doTransfer() {
  if (!actForm.actor || !flow.transferTo.trim()) {
    ElMessage.warning('请选择审批身份并填写接收人')
    return
  }
  acting.value = true
  try {
    await api.approvalTransfer(detailId.value, actForm.actor, flow.transferTo.trim(), flow.transferComment.trim())
    ElMessage.success('已转交，原审批人不再有该待办')
    flow.transferTo = ''
    flow.transferComment = ''
    await refreshDetail()
  } finally {
    acting.value = false
  }
}
async function doCancel() {
  if (!flow.cancelReason.trim()) {
    ElMessage.warning('废止必须填写原因')
    return
  }
  acting.value = true
  try {
    await api.approvalCancel(detailId.value, actForm.actor || '', flow.cancelReason.trim())
    ElMessage.success('流程已废止')
    flow.cancelReason = ''
    await refreshDetail()
    await loadInstances()
  } finally {
    acting.value = false
  }
}
async function refreshDetail() {
  await showDetail({ id: detailId.value } as ApprovalRow)
}
function fmtTime(t?: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

onMounted(() => {
  loadTemplates()
  loadInstances()
  // 从任务中心「发起审批」跳转而来：带入选定合同并直接弹出发起框
  const pending = localStorage.getItem('ch_approval_target')
  if (pending) {
    localStorage.removeItem('ch_approval_target')
    try {
      startTarget.value = JSON.parse(pending)
      startVisible.value = true
    } catch {
      /* ignore */
    }
  }
})
</script>

<style scoped>
.toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.card-title { font-size: 16px; font-weight: 600; margin-right: 12px; }
.card-sub { font-size: 12px; color: #909399; }
.node-chip { margin-right: 10px; font-size: 12px; color: #606266; }
.filters { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; }
.node-block { border: 1px solid #ebeef5; border-radius: 4px; padding: 8px; margin-bottom: 8px; }
.node-row { display: flex; gap: 8px; align-items: center; }
.cond-row { display: flex; gap: 8px; align-items: center; margin-top: 8px; }
.cond-text, .muted { color: #909399; font-size: 12px; }
.cond-tag { margin-right: 6px; }
.comment { font-size: 12px; color: #909399; margin-top: 4px; }
.act-area { margin-top: 8px; }
.flow-row { display: flex; gap: 8px; align-items: center; margin-bottom: 8px; }
</style>
