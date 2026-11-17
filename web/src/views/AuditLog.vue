<template>
  <div>
    <el-card shadow="never">
      <div class="toolbar-line">
        <span class="card-title">审计日志</span>
        <span class="hint">等保「安全审计」自检入口：谁+何时+何事（复用 audit_log 五要素，AK4.4/4.5）</span>
        <el-button type="primary" size="small" @click="exportLog">
          <el-icon><Download /></el-icon>
          导出 Excel
        </el-button>
      </div>

      <!-- 检索：与后端 auditQuery 同一把尺子（action/username LIKE，targetType/dateFrom 精确） -->
      <div class="filters">
        <el-input v-model="q.action" placeholder="动作关键字（如 contract.create）" clearable style="width: 200px" @keyup.enter="load" @clear="load" />
        <el-input v-model="q.targetType" placeholder="对象类型" clearable style="width: 130px" @keyup.enter="load" @clear="load" />
        <el-input v-model="q.username" placeholder="操作人" clearable style="width: 130px" @keyup.enter="load" @clear="load" />
        <el-date-picker v-model="q.dateFrom" type="date" value-format="YYYY-MM-DD" placeholder="起始日期" style="width: 140px" @change="load" />
        <el-button type="primary" plain @click="load"><el-icon><Search /></el-icon>查询</el-button>
        <el-button @click="reset">重置</el-button>
      </div>

      <el-table :data="list" v-loading="loading" border stripe size="small">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="createdAt" label="时间" width="170" />
        <el-table-column prop="username" label="用户" width="120" />
        <el-table-column prop="action" label="动作" min-width="170" />
        <el-table-column prop="targetType" label="对象类型" width="110" />
        <el-table-column prop="targetId" label="对象ID" width="80" />
        <el-table-column prop="ip" label="来源IP" width="130" />
        <el-table-column label="详情" min-width="220">
          <template #default="{ row }"><span class="hint">{{ detailText(row) }}</span></template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && !list.length" :image-size="60" description="暂无审计记录" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api } from '../api'

interface AuditRow {
  id: number
  username: string
  action: string
  targetType: string
  targetId: number
  detail?: Record<string, unknown>
  ip: string
  createdAt: string
}

const list = ref<AuditRow[]>([])
const loading = ref(false)
const q = reactive({ action: '', targetType: '', username: '', dateFrom: '' })

function detailText(row: AuditRow): string {
  if (!row.detail) return ''
  try { return JSON.stringify(row.detail).slice(0, 120) } catch { return '' }
}

async function load() {
  loading.value = true
  try {
    // 后端 auditQuery 额外支持 username LIKE；API 层签名未含该字段，这里直接透传
    const res = await api.http.get<never, any>('/audit-log', {
      params: { action: q.action, targetType: q.targetType, username: q.username, dateFrom: q.dateFrom },
    })
    list.value = res?.data || []
  } finally { loading.value = false }
}

function reset() {
  q.action = ''; q.targetType = ''; q.username = ''; q.dateFrom = ''
  load()
}

function exportLog() {
  window.open(api.auditLogExportUrl({
    action: q.action, targetType: q.targetType, username: q.username, dateFrom: q.dateFrom,
  } as never), '_blank')
}

onMounted(load)
</script>

<style scoped>
.toolbar-line { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
.card-title { font-size: 16px; font-weight: 600; }
.hint { font-size: 12px; color: #909399; flex: 1; }
.filters { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; flex-wrap: wrap; }
</style>
