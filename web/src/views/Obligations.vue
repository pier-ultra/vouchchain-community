<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">义务看板</span>
        <span class="card-sub">复核完成的合同 → 生成义务清单 → 分派负责人 + 截止日 → 临期/逾期跟踪</span>
      </div>
    </div>

    <!-- AC3.4 站内提醒（简版）：逾期/临期聚合提示，邮件通道后续接入 -->
    <el-alert
      v-if="overdueCount || dueSoonCount"
      :type="overdueCount ? 'error' : 'warning'"
      :closable="false"
      style="margin-bottom: 12px"
    >
      <template #title>
        提醒：<b>{{ overdueCount }}</b> 条义务已逾期，<b>{{ dueSoonCount }}</b> 条 7 天内到期
        <el-button link type="primary" @click="query.status = 'overdue'; load()">看逾期</el-button>
        <el-button link type="primary" @click="query.status = 'dueSoon'; load()">看临期</el-button>
      </template>
    </el-alert>

    <!-- 筛选（AC3.7 看板：按合同 / 按负责人 / 按状态） -->
    <div class="filters">
      <el-select v-model="query.fileId" placeholder="全部合同" clearable style="width: 220px" @change="load">
        <el-option v-for="f in files" :key="f.id" :label="`#${f.id} ${f.filename}`" :value="f.id" />
      </el-select>
      <el-input
        v-model="query.owner"
        placeholder="按负责人筛选"
        clearable
        style="width: 180px"
        @keyup.enter="load"
        @clear="load"
      />
      <el-radio-group v-model="query.status" @change="load">
        <el-radio-button value="">全部</el-radio-button>
        <el-radio-button value="pending">待处理</el-radio-button>
        <el-radio-button value="overdue">逾期</el-radio-button>
        <el-radio-button value="dueSoon">7天内到期</el-radio-button>
        <el-radio-button value="done">已完成</el-radio-button>
      </el-radio-group>
      <el-button type="primary" plain @click="load">
        <el-icon><Search /></el-icon>
        刷新
      </el-button>
    </div>

    <el-table :data="filtered" v-loading="loading" border stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column label="合同" width="160">
        <template #default="{ row }">
          <span class="dim">#{{ row.fileId }} {{ fileMap[row.fileId] || '' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="90">
        <template #default="{ row }">
          <el-tag :type="typeTagType(row.type)" size="small">{{ row.type }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="义务描述（谁+做什么+何时）" min-width="260" show-overflow-tooltip />
      <el-table-column label="负责人" width="150">
        <template #default="{ row }">
          <el-input
            v-model="row.owner"
            size="small"
            placeholder="待分派"
            @change="saveField(row, { owner: row.owner })"
          />
        </template>
      </el-table-column>
      <el-table-column label="截止日期" width="150">
        <template #default="{ row }">
          <el-date-picker
            v-model="row.dueDate"
            type="date"
            size="small"
            value-format="YYYY-MM-DD"
            placeholder="未设置"
            style="width: 130px"
            @change="saveField(row, { dueDate: row.dueDate || '' })"
          />
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.status === 'done'" type="success" size="small">已完成</el-tag>
          <el-tag v-else-if="row.overdue" type="danger" size="small" effect="dark">逾期</el-tag>
          <el-tag v-else-if="row.dueSoon" type="warning" size="small">7天内到期</el-tag>
          <el-tag v-else type="info" size="small">待处理</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="依据原文" min-width="220">
        <template #default="{ row }">
          <blockquote class="quote">{{ row.sourceQuote }}</blockquote>
        </template>
      </el-table-column>
      <el-table-column label="证据附件" width="140">
        <template #default="{ row }">
          <a v-if="row.evidenceName" :href="api.evidenceUrl(row.id)" target="_blank" class="ev-link">
            <el-icon><Paperclip /></el-icon>
            {{ row.evidenceName }}
          </a>
          <span v-else class="dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="110" fixed="right">
        <template #default="{ row }">
          <el-button v-if="row.status === 'pending'" link type="success" @click="openComplete(row)">标记完成</el-button>
          <el-button v-else link type="warning" @click="reopen(row)">重新打开</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- AC3.6 完成反馈：可选上传证据附件（留痕） -->
    <el-dialog v-model="completeVisible" title="标记完成（可上传证据附件）" width="440px">
      <div class="complete-row">
        <el-input v-model="completeRemark" placeholder="完成说明（可选）" style="flex: 1" />
      </div>
      <el-upload :show-file-list="false" :auto-upload="false" :on-change="onEvidencePick" style="margin-top: 12px">
        <el-button size="small">
          <el-icon><Paperclip /></el-icon>
          {{ evidenceFile ? evidenceFile.name : '选择证据附件（可选）' }}
        </el-button>
      </el-upload>
      <template #footer>
        <el-button @click="completeVisible = false">取消</el-button>
        <el-button type="primary" :loading="completing" @click="confirmComplete">确认完成</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import type { UploadFile } from 'element-plus'
import { api, type ContractFile, type Obligation } from '../api'

const list = ref<Obligation[]>([])
const files = ref<ContractFile[]>([])
const fileMap = ref<Record<number, string>>({})
const loading = ref(false)

const query = reactive({ fileId: undefined as number | undefined, owner: '', status: '' })

// 状态过滤：overdue/dueSoon 是计算态，前端在 pending 基础上再筛
const filtered = computed(() => {
  if (query.status === 'overdue') return list.value.filter((o) => o.status === 'pending' && o.overdue)
  if (query.status === 'dueSoon') return list.value.filter((o) => o.status === 'pending' && o.dueSoon && !o.overdue)
  if (query.status) return list.value.filter((o) => o.status === query.status)
  return list.value
})

async function load() {
  loading.value = true
  try {
    const res = await api.obligations({
      owner: query.owner || undefined,
      fileId: query.fileId || undefined,
    })
    list.value = res.data || []
    const fres = await api.contractFiles()
    files.value = (fres.data || []).filter((f) => f.reviewState === 'complete')
    const m: Record<number, string> = {}
    for (const f of files.value) m[f.id] = f.filename
    fileMap.value = m
  } finally {
    loading.value = false
  }
}

async function saveField(row: Obligation, d: { owner?: string; dueDate?: string }) {
  await api.updateObligation(row.id, d)
  ElMessage.success('已保存（留痕）')
  await load()
}

// ---------- AC3.4 提醒聚合 ----------
const overdueCount = computed(() => list.value.filter((o) => o.status === 'pending' && o.overdue).length)
const dueSoonCount = computed(() => list.value.filter((o) => o.status === 'pending' && o.dueSoon && !o.overdue).length)

// ---------- AC3.6 完成反馈 + 证据附件 ----------
const completeVisible = ref(false)
const completing = ref(false)
const completeRemark = ref('')
const evidenceFile = ref<File | null>(null)
const completingRow = ref<Obligation | null>(null)

function openComplete(row: Obligation) {
  completingRow.value = row
  completeRemark.value = ''
  evidenceFile.value = null
  completeVisible.value = true
}

function onEvidencePick(uploadFile: UploadFile) {
  evidenceFile.value = uploadFile.raw || null
}

async function confirmComplete() {
  const row = completingRow.value
  if (!row) return
  completing.value = true
  try {
    if (evidenceFile.value) {
      await api.uploadEvidence(row.id, evidenceFile.value)
    }
    await api.updateObligation(row.id, {
      status: 'done',
      ...(completeRemark.value ? { remark: completeRemark.value } : {}),
    })
    ElMessage.success('已完成（留痕）')
    completeVisible.value = false
    await load()
  } finally {
    completing.value = false
  }
}

async function reopen(row: Obligation) {
  await api.updateObligation(row.id, { status: 'pending' })
  ElMessage.success('已重新打开')
  await load()
}

function typeTagType(t: string): 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  const m: Record<string, 'primary' | 'success' | 'warning' | 'danger' | 'info'> = {
    付款: 'success', 交付: 'primary', 报告: 'warning', 续约: 'danger',
  }
  return m[t] || 'info'
}

onMounted(load)
</script>

<style scoped>
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
.filters {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.dim {
  color: #909399;
  font-size: 12px;
}
.quote {
  margin: 0;
  padding: 4px 8px;
  border-left: 3px solid var(--el-text-color-secondary);
  background: var(--ch-surface-3);
  color: var(--el-text-color-secondary);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.complete-row {
  display: flex;
  gap: 8px;
}
.ev-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #409eff;
  text-decoration: none;
  word-break: break-all;
}
</style>
