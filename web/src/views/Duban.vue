<template>
  <div>
    <!-- 新建督办（R-D1） -->
    <el-card shadow="never" class="mb">
      <template #header><b>新建督办任务</b></template>
      <el-form :inline="true">
        <el-form-item label="标题"><el-input v-model="form.title" placeholder="会议决议/领导批示事项" style="width: 220px" /></el-form-item>
        <el-form-item label="来源类型">
          <el-select v-model="form.sourceType" style="width: 130px">
            <el-option label="会议决议" value="meeting" />
            <el-option label="领导批示" value="directive" />
            <el-option label="其他" value="other" />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级">
          <el-select v-model="form.priority" style="width: 110px">
            <el-option label="高" value="high" />
            <el-option label="中" value="mid" />
            <el-option label="低" value="low" />
          </el-select>
        </el-form-item>
        <el-form-item label="期限"><el-date-picker v-model="form.deadline" type="date" value-format="YYYY-MM-DD" style="width: 140px" /></el-form-item>
        <el-form-item label="责任人"><el-input v-model="form.owner" style="width: 110px" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" style="width: 150px" /></el-form-item>
        <el-form-item><el-button type="primary" :loading="creating" @click="create">创建督办</el-button></el-form-item>
      </el-form>
    </el-card>

    <!-- 看板过滤（R-D2） -->
    <div class="toolbar-line">
      <el-select v-model="filter.sourceType" clearable placeholder="来源" style="width: 130px" @change="loadBoard">
        <el-option label="会议决议" value="meeting" />
        <el-option label="领导批示" value="directive" />
        <el-option label="其他" value="other" />
      </el-select>
      <el-select v-model="filter.priority" clearable placeholder="优先级" style="width: 120px" @change="loadBoard">
        <el-option label="高" value="high" />
        <el-option label="中" value="mid" />
        <el-option label="低" value="low" />
      </el-select>
      <el-button @click="loadBoard" :loading="boardLoading">刷新看板</el-button>
    </div>

    <!-- 看板三列 -->
    <div class="board">
      <div v-for="col in cols" :key="col.key" class="board-col">
        <div class="col-head" :class="col.key">
          {{ col.label }}<span class="cnt">{{ board[col.key]?.length || 0 }}</span>
        </div>
        <div class="col-body">
          <el-card v-for="t in board[col.key] || []" :key="t.id" shadow="hover" class="task-card">
            <div class="task-title">{{ t.title }}</div>
            <div class="task-meta">
              <el-tag size="small" :type="prioType(t.priority)">{{ prioLabel(t.priority) }}</el-tag>
              <el-tag size="small" type="info">{{ srcLabel(t.source_type) }}</el-tag>
            </div>
            <div class="task-meta hint">
              <span v-if="t.owner">责任:{{ t.owner }}</span>
              <span v-if="t.deadline">期限:{{ (t.deadline || '').slice(0, 10) }}</span>
              <span v-if="t.remind_cnt > 0">催办×{{ t.remind_cnt }}</span>
            </div>
            <div class="task-actions">
              <el-button v-if="t.derived_status !== 'done'" link type="primary" size="small" @click="remind(t)">催办</el-button>
              <el-button v-if="t.derived_status !== 'done'" link type="success" size="small" @click="done(t)">完成</el-button>
              <el-button link size="small" @click="showLog(t)">留痕</el-button>
            </div>
          </el-card>
          <el-empty v-if="!(board[col.key] || []).length" :image-size="60" description="暂无" />
        </div>
      </div>
    </div>

    <!-- 留痕对话框 -->
    <el-dialog v-model="logVisible" :title="`留痕 · ${cur.title || ''}`" width="520px">
      <el-timeline>
        <el-timeline-item v-for="(l, i) in logs" :key="i" :timestamp="(l.created_at || '').slice(0, 19)" placement="top">
          <span class="hint">{{ l.actor }}</span> <b>{{ actLabel(l.action) }}</b>
          <div v-if="l.note" class="hint">{{ l.note }}</div>
        </el-timeline-item>
      </el-timeline>
      <el-empty v-if="!logs.length" :image-size="50" description="暂无留痕" />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const cols = [
  { key: 'todo', label: '待办/进行中' },
  { key: 'overdue', label: '逾期' },
  { key: 'done', label: '已完成' },
]
const form = reactive({ title: '', sourceType: 'meeting', priority: 'mid', deadline: '', owner: '', remark: '' })
const filter = reactive({ sourceType: '', priority: '' })
const board = ref<Record<string, any[]>>({})
const boardLoading = ref(false)
const creating = ref(false)
const logVisible = ref(false)
const logs = ref<any[]>([])
const cur = ref<any>({})

function prioLabel(p: string) { return p === 'high' ? '高' : p === 'low' ? '低' : '中' }
function prioType(p: string) { return p === 'high' ? 'danger' : p === 'low' ? 'info' : 'warning' }
function srcLabel(s: string) { return s === 'meeting' ? '会议决议' : s === 'directive' ? '领导批示' : '其他' }
function actLabel(a: string) { return a === 'create' ? '创建' : a === 'remind' ? '催办' : a === 'done' ? '完成' : a }

async function loadBoard() {
  boardLoading.value = true
  try {
    const q: string[] = []
    if (filter.sourceType) q.push(`sourceType=${filter.sourceType}`)
    if (filter.priority) q.push(`priority=${filter.priority}`)
    const res = await api.http.get<never, any>(`/duban/board${q.length ? '?' + q.join('&') : ''}`)
    board.value = res.data || {}
  } finally { boardLoading.value = false }
}

async function create() {
  if (!form.title) { ElMessage.warning('请输入标题'); return }
  creating.value = true
  try {
    await api.http.post<never, any>('/duban/tasks', { ...form })
    ElMessage.success('督办已创建')
    form.title = ''; form.owner = ''; form.remark = ''
    loadBoard()
  } finally { creating.value = false }
}

async function remind(t: any) {
  await api.http.post<never, any>(`/duban/tasks/${t.id}/remind`, { note: '系统催办' })
  ElMessage.success('已催办')
  loadBoard()
}

async function done(t: any) {
  await api.http.post<never, any>(`/duban/tasks/${t.id}/done`, { note: '已完成' })
  ElMessage.success('已完成')
  loadBoard()
}

async function showLog(t: any) {
  cur.value = t
  const res = await api.http.get<never, any>(`/duban/tasks/${t.id}/log`)
  logs.value = res.data || []
  logVisible.value = true
}

onMounted(loadBoard)
</script>

<style scoped>
.mb { margin-bottom: 12px; }
.toolbar-line { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; }
.board { display: flex; gap: 12px; align-items: flex-start; }
.board-col { flex: 1; background: var(--ch-surface-3); border-radius: 6px; padding: 8px; }
.col-head { font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 6px; }
.col-head.overdue { color: var(--el-color-danger); }
.col-head.done { color: var(--el-color-success); }
.cnt { font-size: 12px; color: #909399; }
.task-card { margin-bottom: 8px; }
.task-title { font-weight: 600; margin-bottom: 6px; }
.task-meta { display: flex; gap: 6px; margin-bottom: 4px; flex-wrap: wrap; }
.task-actions { margin-top: 6px; }
.hint { font-size: 12px; color: #909399; }
</style>
