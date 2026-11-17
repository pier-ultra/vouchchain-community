<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">任务中心</span>
        <span class="card-sub">
          上传 → 文本解析 → AI 条款抽取 → 向量化（知识库阶段接入）
          <el-tag v-if="wsOnline" type="success" effect="plain" style="margin-left: 8px">实时推送已连接</el-tag>
          <el-tag v-else type="info" effect="plain" style="margin-left: 8px">轮询模式</el-tag>
        </span>
      </div>
    </div>

    <el-alert
      type="info"
      :closable="false"
      style="margin-bottom: 14px"
      title="支持 DOCX 与文字版 PDF（≤20MB）"
      description="长合同自动按段落分块（约 3000 字/块）逐块抽取；扫描件/图片版 PDF 暂不支持（OCR 后续接入）"
    />

    <!-- AC1.8 抽取模板选择 + 上传 -->
    <div class="upload-row">
      <el-select v-model="uploadTemplate" style="width: 160px">
        <el-option label="通用模板" value="general" />
        <el-option label="采购合同" value="purchase" />
        <el-option label="销售合同" value="sales" />
        <el-option label="租赁合同" value="lease" />
      </el-select>
      <el-upload
        drag
        class="upload-area"
        :show-file-list="false"
        :auto-upload="false"
        accept=".pdf,.docx"
        :on-change="onFilePick"
      >
        <el-icon style="font-size: 42px; color: #409eff"><UploadFilled /></el-icon>
        <div class="el-upload__text">拖拽文件到此处，或 <em>点击选择</em>（左侧选模板，上传后自动开始流水线）</div>
      </el-upload>
    </div>

    <el-table :data="files" v-loading="loading" border stripe style="margin-top: 16px">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="filename" label="文件名" min-width="170">
        <template #default="{ row }">
          <span>{{ row.filename }}</span>
          <el-tag v-if="row.versionNo > 1" size="small" type="warning" style="margin-left: 6px">v{{ row.versionNo }}</el-tag>
        </template>
      </el-table-column>

      <!-- 流水线阶段（任务中心核心：每阶段独立状态与进度） -->
      <el-table-column label="流水线进度" min-width="330">
        <template #default="{ row }">
          <div class="pipe">
            <div v-for="s in row.pipeline || []" :key="s.key" class="pipe-item">
              <el-tag
                :type="stageTagType(s.status)"
                size="small"
                :effect="s.status === 'running' ? 'dark' : 'light'"
              >
                {{ s.label }}
              </el-tag>
              <el-progress
                :percentage="s.progress"
                :stroke-width="6"
                :status="s.status === 'failed' ? 'exception' : s.status === 'done' ? 'success' : undefined"
                class="pipe-bar"
              />
              <span class="pipe-msg">{{ s.msg || stageDefaultMsg(s) }}</span>
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="总进度" width="110">
        <template #default="{ row }">
          <el-progress type="circle" :percentage="row.progress || 0" :width="54" :stroke-width="6"
            :status="row.status === 'failed' ? 'exception' : row.status === 'done' ? 'success' : undefined" />
        </template>
      </el-table-column>

      <el-table-column label="上传时间" width="160">
        <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-tag v-if="row.reviewState === 'complete'" type="success" size="small" style="margin-right: 4px">复核完成</el-tag>
          <el-tag v-else-if="row.approvalState === 'running'" type="warning" size="small" style="margin-right: 4px">审批中</el-tag>
          <el-tag v-else-if="row.approvalState === 'approved'" type="success" size="small" style="margin-right: 4px">已批准</el-tag>
          <el-tag v-else-if="row.approvalState === 'rejected'" type="danger" size="small" style="margin-right: 4px">已驳回</el-tag>
          <el-button v-if="row.status === 'done'" link type="primary" @click="showClauses(row)">条款</el-button>
          <el-button v-if="row.status === 'done'" link type="primary" @click="showVersions(row)">版本</el-button>
          <el-button link type="primary" @click="openNewVersion(row)">升版</el-button>
          <el-button v-if="row.status === 'done' && row.reviewState === 'complete' && row.approvalState === 'none'" link type="success" @click="startApproval(row)">发起审批</el-button>
          <el-button v-if="row.status === 'failed'" link type="warning" @click="retry(row)">重试</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 条款复核抽屉（AC2.1 逐条展示 / AC2.2 修正 / AC2.3 确认与疑义 / AC2.6 完成门槛） -->
    <el-drawer v-model="drawerVisible" :title="`抽取结果与复核：${activeFile?.filename || ''}`" size="66%">
      <div class="drawer-head">
        <span class="review-stat">
          复核进度：已处理 {{ reviewedCount }} / {{ clauses.length }} 条
          <el-tag v-if="activeFile?.reviewState === 'complete'" type="success" size="small" style="margin-left: 8px">复核完成</el-tag>
        </span>
        <div class="drawer-actions">
          <!-- AC2.4 批量复核 -->
          <el-button size="small" type="success" plain :disabled="!selected.length" @click="batchReview(selected, 'confirmed')">
            批量确认（{{ selected.length }}）
          </el-button>
          <el-button size="small" type="warning" plain :disabled="!selected.length" @click="batchReview(selected, 'doubt')">
            批量疑义（{{ selected.length }}）
          </el-button>
          <el-dropdown @command="exportClauses">
            <el-button size="small">
              <el-icon><Download /></el-icon>
              导出
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="xlsx">Excel</el-dropdown-item>
                <el-dropdown-item command="json">JSON</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button
            v-if="activeFile?.reviewState === 'complete'"
            type="success"
            size="small"
            :loading="generating"
            @click="genObligations"
          >
            生成义务清单
          </el-button>
          <el-button
            type="primary"
            size="small"
            :disabled="reviewedCount < clauses.length || clauses.length === 0 || activeFile?.reviewState === 'complete'"
            :loading="completing"
            @click="completeReview"
          >
            完成复核（进入下一环节）
          </el-button>
        </div>
      </div>
      <el-table :data="clauses" v-loading="clausesLoading" border @selection-change="onSelectChange">
        <el-table-column type="selection" width="42" />
        <el-table-column label="类型" width="96">
          <template #default="{ row }">
            <el-tag :type="typeTagType(row.type)">{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="条款要点（可修正）" min-width="210">
          <template #default="{ row }">
            <span>{{ row.summary }}</span>
            <el-button link type="primary" style="margin-left: 6px" @click="openEdit(row)">修正</el-button>
          </template>
        </el-table-column>
        <el-table-column label="原文引用" min-width="240">
          <template #default="{ row }">
            <blockquote class="quote">{{ row.quote }}</blockquote>
          </template>
        </el-table-column>
        <el-table-column label="风险" width="80">
          <template #default="{ row }">
            <el-tag :type="riskTagType(row.risk)" effect="plain">{{ riskLabel(row.risk) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="复核" width="210">
          <template #default="{ row }">
            <el-tag v-if="row.reviewStatus === 'confirmed'" type="success" size="small">已确认</el-tag>
            <el-tag v-else-if="row.reviewStatus === 'doubt'" type="warning" size="small">有疑义</el-tag>
            <el-tag v-else type="info" size="small">待复核</el-tag>
            <div class="review-btns">
              <el-button size="small" type="success" plain @click="mark(row, 'confirmed')">确认</el-button>
              <el-button size="small" type="warning" plain @click="mark(row, 'doubt')">疑义</el-button>
            </div>
            <div v-if="row.reviewRemark" class="remark">{{ row.reviewRemark }}</div>
          </template>
        </el-table-column>
        <el-table-column label="原文" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="locateOriginal(row)">定位原文</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>

    <!-- AC6.2 版本历史 -->
    <el-dialog v-model="versionsVisible" :title="`版本历史：${versionsBase?.filename || ''}`" width="640px">
      <el-table :data="versions" v-loading="versionsLoading" border size="small">
        <el-table-column label="版本" width="70">
          <template #default="{ row }">
            <el-tag size="small" :type="row.id === versionsBase?.id ? 'primary' : 'info'">v{{ row.versionNo }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="filename" label="文件名" min-width="180" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.status === 'done'" type="success" size="small">已完成</el-tag>
            <el-tag v-else-if="row.status === 'extracting'" type="warning" size="small">抽取中</el-tag>
            <el-tag v-else-if="row.status === 'failed'" type="danger" size="small">失败</el-tag>
            <el-tag v-else type="info" size="small">待处理</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="上传时间" width="160">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- AC6.2 升版上传 -->
    <el-dialog v-model="newVersionVisible" :title="`上传新版本（基于：${versionBase?.filename || ''}）`" width="440px">
      <el-upload drag :show-file-list="false" :auto-upload="false" accept=".pdf,.docx" :on-change="onVersionPick">
        <el-icon style="font-size: 32px; color: #409eff"><UploadFilled /></el-icon>
        <div class="el-upload__text">选择新版本文件（DOCX / 文字版 PDF）</div>
      </el-upload>
      <el-alert v-if="versionPicked" type="success" :closable="false" style="margin-top: 10px"
        :title="`已选择：${versionPicked.name}，上传后自动进入流水线`" />
    </el-dialog>

    <!-- AC1.5 原文定位：全文展示 + 条款原文高亮滚动 -->
    <el-dialog v-model="textDialogVisible" :title="`合同原文：${textFile?.filename || ''}`" width="72%" top="4vh">
      <div ref="textBodyRef" class="original-text">
        <template v-for="(seg, i) in textSegments" :key="i">
          <span :ref="seg.hit ? 'hitRef' : undefined" :class="seg.hit ? 'hit' : ''">{{ seg.text }}</span>
        </template>
      </div>
    </el-dialog>

    <!-- 条款修正弹窗（AC2.2：保存留痕 → 审计轨迹） -->
    <el-dialog v-model="editVisible" title="修正条款" width="520px">
      <el-form label-width="90px">
        <el-form-item label="条款要点">
          <el-input v-model="editForm.summary" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item label="风险等级">
          <el-select v-model="editForm.risk" style="width: 180px">
            <el-option label="高" value="high" />
            <el-option label="中" value="medium" />
            <el-option label="低" value="low" />
            <el-option label="无" value="none" />
          </el-select>
        </el-form-item>
        <el-form-item label="疑义备注">
          <el-input v-model="editForm.reviewRemark" type="textarea" :rows="2" placeholder="标记疑义时可填说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveEdit">保存（留痕）</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import type { UploadFile } from 'element-plus'
import { api, type ContractClause, type ContractFile, type PipelineStage } from '../api'

const files = ref<ContractFile[]>([])
const loading = ref(false)
const drawerVisible = ref(false)
const clausesLoading = ref(false)
const clauses = ref<ContractClause[]>([])
const activeFile = ref<ContractFile | null>(null)
const editVisible = ref(false)
const saving = ref(false)
const completing = ref(false)
const generating = ref(false)
const editingClause = ref<ContractClause | null>(null)
const editForm = reactive({ summary: '', risk: 'none', reviewRemark: '' })

// AC2.6：已复核条目数（确认或疑义都算已处理）
const reviewedCount = computed(() => clauses.value.filter((c) => c.reviewStatus !== 'unconfirmed').length)

// AC1.7 抽取结果导出（JSON / Excel，服务端生成下载）
function exportClauses(format: 'json' | 'xlsx') {
  if (!activeFile.value) return
  window.open(api.exportClausesUrl(activeFile.value.id, format), '_blank')
}

// AC3.1 生成义务清单（仅复核完成后可用）
async function genObligations() {
  if (!activeFile.value) return
  generating.value = true
  try {
    const res = await api.generateObligations(activeFile.value.id)
    ElMessage.success(`义务清单已生成（${res.data?.count ?? 0} 条），去「义务看板」分派`)
  } finally {
    generating.value = false
  }
}

// ---------- AC6.4 发起审批（跳转审批中心并带入合同） ----------
function startApproval(row: ContractFile) {
  localStorage.setItem('ch_approval_target', JSON.stringify({ id: row.id, filename: row.filename }))
  window.location.hash = '#/approvals'
}

// ---------- AC6.2 版本历史 / 升版 ----------
const versionsVisible = ref(false)
const versionsLoading = ref(false)
const versions = ref<ContractFile[]>([])
const versionsBase = ref<ContractFile | null>(null)
const newVersionVisible = ref(false)
const versionBase = ref<ContractFile | null>(null)
const versionPicked = ref<File | null>(null)

async function showVersions(row: ContractFile) {
  versionsBase.value = row
  versions.value = []
  versionsVisible.value = true
  versionsLoading.value = true
  try {
    const res = await api.fileVersions(row.id)
    versions.value = res.data || []
  } finally {
    versionsLoading.value = false
  }
}

function openNewVersion(row: ContractFile) {
  versionBase.value = row
  versionPicked.value = null
  newVersionVisible.value = true
}

async function onVersionPick(uploadFile: UploadFile) {
  const raw = uploadFile.raw
  if (!raw || !versionBase.value) return
  if (!/\.(pdf|docx)$/i.test(raw.name)) {
    ElMessage.error('仅支持 PDF / DOCX 格式')
    return
  }
  versionPicked.value = raw
  const res = await api.uploadContractFile(raw, versionBase.value.template, versionBase.value.id)
  ElMessage.success(`新版本已上传（任务 #${res.data?.id}，v${(versions.value.at(-1)?.versionNo || 1) + 1}）`)
  newVersionVisible.value = false
  await load()
}

// ---------- AC1.5 原文定位 ----------
const textDialogVisible = ref(false)
const textFile = ref<{ id: number; filename: string } | null>(null)
const textSegments = ref<{ text: string; hit: boolean }[]>([])
const hitRef = ref<HTMLElement[] | null>(null)
const textBodyRef = ref<HTMLElement | null>(null)

async function locateOriginal(row: ContractClause) {
  if (!activeFile.value) return
  const res = await api.fileText(activeFile.value.id)
  const full = res.data?.text || ''
  if (!full) {
    ElMessage.warning('该文件缺解析全文（旧任务请重试抽取）')
    return
  }
  textFile.value = { id: res.data!.id, filename: res.data!.filename }
  // 按 quote 首次出现位置切三段，命中段高亮
  const q = row.quote.trim()
  const idx = q ? full.indexOf(q) : -1
  if (idx >= 0) {
    textSegments.value = [
      { text: full.slice(0, idx), hit: false },
      { text: q, hit: true },
      { text: full.slice(idx + q.length), hit: false },
    ]
  } else {
    // 精确匹配失败（模型可能轻微改写）：退化为整段展示
    textSegments.value = [{ text: full, hit: false }]
    ElMessage.info('未精确匹配到原文位置，已展示全文供人工核对')
  }
  textDialogVisible.value = true
  setTimeout(() => {
    const el = document.querySelector('.original-text .hit')
    el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }, 300)
}

const wsOnline = ref(false)
let ws: WebSocket | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null
let wsRetryTimer: ReturnType<typeof setTimeout> | null = null

// ---------- 数据加载 ----------
async function load() {
  loading.value = true
  try {
    const res = await api.contractFiles()
    files.value = res.data || []
  } finally {
    loading.value = false
  }
}

// ---------- WebSocket 实时进度（断线自动重连 + 轮询兜底） ----------
function connectWs() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  // 浏览器 WebSocket 无法自定义请求头，SSO 态把 token 走 query 带给后端鉴权中间件
  const token = localStorage.getItem('ch_token') || ''
  const query = token ? `?token=${encodeURIComponent(token)}` : ''
  try {
    ws = new WebSocket(`${proto}://${location.host}/api/ws/tasks${query}`)
  } catch {
    startPolling()
    return
  }
  ws.onopen = () => {
    wsOnline.value = true
    stopPolling()
  }
  ws.onmessage = (ev) => {
    try {
      const evt = JSON.parse(ev.data)
      if (evt.type === 'task') applyTaskUpdate(evt.data)
    } catch {
      /* ignore */
    }
  }
  ws.onclose = () => {
    wsOnline.value = false
    startPolling()
    wsRetryTimer = setTimeout(connectWs, 5000)
  }
  ws.onerror = () => ws?.close()
}

function applyTaskUpdate(d: Partial<ContractFile> & { id?: number }) {
  const idx = files.value.findIndex((f) => f.id === d.id)
  if (idx < 0) {
    load()
    return
  }
  const cur = files.value[idx]
  if (d.status !== undefined) cur.status = d.status
  if (d.progress !== undefined) cur.progress = d.progress
  if (d.stage !== undefined) cur.stage = d.stage
  if (d.pipeline !== undefined) cur.pipeline = d.pipeline as PipelineStage[]
  if (d.errorMsg !== undefined) cur.errorMsg = d.errorMsg
}

function startPolling() {
  if (pollTimer) return
  pollTimer = setInterval(load, 3000)
}
function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// ---------- 上传 / 重试 ----------
const uploadTemplate = ref('general') // AC1.8 抽取模板
const selected = ref<ContractClause[]>([]) // AC2.4 批量复核选中项

function onSelectChange(rows: ContractClause[]) {
  selected.value = rows
}

async function batchReview(rows: ContractClause[], status: 'confirmed' | 'doubt') {
  const ids = rows.map((r) => r.id)
  if (!ids.length) return
  await api.batchReview(ids, status)
  ElMessage.success(`已批量处理 ${ids.length} 条（留痕）`)
  await showClauses(activeFile.value!)
}

async function onFilePick(uploadFile: UploadFile) {
  const raw = uploadFile.raw
  if (!raw) return
  if (!/\.(pdf|docx)$/i.test(raw.name)) {
    ElMessage.error('仅支持 PDF / DOCX 格式')
    return
  }
  const res = await api.uploadContractFile(raw, uploadTemplate.value)
  ElMessage.success(`已上传，流水线开始（任务 #${res.data?.id}）`)
  await load()
}

async function retry(row: ContractFile) {
  await api.retryContractFile(row.id)
  ElMessage.success('已重新触发流水线')
  await load()
}

// ---------- 条款复核 ----------
async function showClauses(row: ContractFile) {
  activeFile.value = row
  clauses.value = []
  drawerVisible.value = true
  clausesLoading.value = true
  try {
    const res = await api.contractFileClauses(row.id)
    clauses.value = res.data || []
  } finally {
    clausesLoading.value = false
  }
}

async function mark(row: ContractClause, status: 'confirmed' | 'doubt') {
  await api.updateClause(row.id, { reviewStatus: status })
  row.reviewStatus = status
  ElMessage.success(status === 'confirmed' ? '已确认（留痕）' : '已标记疑义（留痕）')
}

// AC2.6 复核完成：全部确认/疑义后才能点；后端二次校验
async function completeReview() {
  if (!activeFile.value) return
  completing.value = true
  try {
    await api.completeReview(activeFile.value.id)
    activeFile.value.reviewState = 'complete'
    ElMessage.success('复核完成，已进入下一环节（留痕）')
  } finally {
    completing.value = false
  }
}

function openEdit(row: ContractClause) {
  editingClause.value = row
  editForm.summary = row.summary
  editForm.risk = row.risk
  editForm.reviewRemark = row.reviewRemark || ''
  editVisible.value = true
}

async function saveEdit() {
  if (!editingClause.value) return
  saving.value = true
  try {
    await api.updateClause(editingClause.value.id, {
      summary: editForm.summary,
      risk: editForm.risk,
      reviewRemark: editForm.reviewRemark,
      reviewStatus: 'confirmed',
    })
    editingClause.value.summary = editForm.summary
    editingClause.value.risk = editForm.risk
    editingClause.value.reviewRemark = editForm.reviewRemark
    editingClause.value.reviewStatus = 'confirmed'
    ElMessage.success('已修正并留痕')
    editVisible.value = false
  } finally {
    saving.value = false
  }
}

// ---------- 展示辅助 ----------
function stageTagType(s: string): 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  if (s === 'done') return 'success'
  if (s === 'running') return 'warning'
  if (s === 'failed') return 'danger'
  return 'info'
}
function stageDefaultMsg(s: PipelineStage) {
  if (s.key === 'vectorize' && s.status === 'planned') return '知识库阶段接入'
  return ''
}
function typeTagType(t: string): 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  const m: Record<string, 'primary' | 'success' | 'warning' | 'danger' | 'info'> = {
    主体: 'primary', 金额: 'success', 违约: 'danger', 保密: 'warning',
  }
  return m[t] || 'info'
}
function riskTagType(r: string): 'danger' | 'warning' | 'info' | 'success' {
  if (r === 'high') return 'danger'
  if (r === 'medium') return 'warning'
  if (r === 'low') return 'info'
  return 'success'
}
function riskLabel(r: string) {
  const m: Record<string, string> = { high: '高', medium: '中', low: '低', none: '无' }
  return m[r] || r
}
function fmtTime(t?: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

onMounted(() => {
  load()
  connectWs()
})
onBeforeUnmount(() => {
  ws?.close()
  stopPolling()
  if (wsRetryTimer) clearTimeout(wsRetryTimer)
})
</script>

<style scoped>
.upload-row {
  display: flex;
  align-items: stretch;
  gap: 12px;
}
.upload-area {
  flex: 1;
}
.toolbar {
  margin-bottom: 14px;
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
.pipe {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.pipe-item {
  display: flex;
  align-items: center;
  gap: 8px;
}
.pipe-item .el-tag {
  width: 86px;
  justify-content: center;
}
.pipe-bar {
  flex: 1;
}
.pipe-msg {
  width: 150px;
  font-size: 12px;
  color: #909399;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.quote {
  margin: 0;
  padding: 6px 10px;
  border-left: 3px solid var(--el-color-primary);
  background: var(--ch-surface-3);
  color: var(--el-text-color-regular);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.drawer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.review-stat {
  font-size: 13px;
  color: #606266;
}
.original-text {
  max-height: 70vh;
  overflow: auto;
  line-height: 2;
  font-size: 13px;
  color: #303133;
  white-space: pre-wrap;
  word-break: break-word;
  padding: 4px 8px;
}
.original-text .hit {
  background: rgba(230, 162, 60, 0.16);
  border-bottom: 2px solid var(--el-color-warning);
  font-weight: 600;
}
.review-btns {
  margin-top: 4px;
  display: flex;
  gap: 4px;
}
.remark {
  margin-top: 4px;
  font-size: 12px;
  color: #e6a23c;
  white-space: pre-wrap;
}
</style>
