<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">签署管理</span>
        <span class="card-sub">AC7：适配层对接外部电子签（API 对接，不内嵌签提供方代码）；MVP 内置 mock 提供方演示状态机</span>
      </div>
      <el-button type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon>
        发起签署
      </el-button>
    </div>

    <el-alert
      type="info"
      :closable="false"
      style="margin-bottom: 14px"
      title="mock 提供方：发起即送达（sent），点「查询状态」模拟对方签署完成（signed）"
      description="真实提供方（DocuSeal / 商业 CA）在「模型配置」登记 esign 类别后启用；未配置时任务会明确置失败，不假装成功"
    />

    <el-table :data="tasks" v-loading="loading" border stripe>
      <el-table-column prop="id" label="任务ID" width="80" />
      <el-table-column label="合同" width="200">
        <template #default="{ row }">#{{ row.fileId }} {{ fileMap[row.fileId] || '' }}</template>
      </el-table-column>
      <el-table-column prop="provider" label="提供方" width="110" />
      <el-table-column label="签署状态" width="110">
        <template #default="{ row }">
          <el-tag v-if="row.status === 'signed'" type="success">已签署</el-tag>
          <el-tag v-else-if="row.status === 'sent'" type="warning">已送达</el-tag>
          <el-tag v-else-if="row.status === 'failed'" type="danger">失败</el-tag>
          <el-tag v-else type="info">已创建</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="失败原因" min-width="220">
        <template #default="{ row }">
          <span v-if="row.failMsg" class="err">{{ row.failMsg }}</span>
          <span v-else class="dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="外部单号" width="140">
        <template #default="{ row }">{{ row.externalId || '—' }}</template>
      </el-table-column>
      <el-table-column label="发起时间" width="160">
        <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="110" fixed="right">
        <template #default="{ row }">
          <el-button v-if="row.status === 'sent'" link type="primary" @click="poll(row)">查询状态</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="发起签署" width="480px">
      <el-form label-width="90px">
        <el-form-item label="选择合同" required>
          <el-select v-model="form.fileId" placeholder="已抽取完成的合同" style="width: 100%">
            <el-option v-for="f in files" :key="f.id" :label="`#${f.id} ${f.filename}`" :value="f.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="提供方">
          <el-select v-model="form.provider" style="width: 100%">
            <el-option label="mock（本地模拟）" value="mock" />
            <el-option label="DocuSeal（外部 API）" value="docuseal" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">发起</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type ContractFile, type SignTask } from '../api'

const tasks = ref<SignTask[]>([])
const files = ref<ContractFile[]>([])
const fileMap = ref<Record<number, string>>({})
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const form = reactive({ fileId: undefined as number | undefined, provider: 'mock' })

async function load() {
  loading.value = true
  try {
    const res = await api.signTasks()
    tasks.value = res.data || []
    const fres = await api.contractFiles()
    files.value = (fres.data || []).filter((f) => f.status === 'done')
    const m: Record<number, string> = {}
    for (const f of files.value) m[f.id] = f.filename
    fileMap.value = m
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.fileId = undefined
  form.provider = 'mock'
  dialogVisible.value = true
}

async function save() {
  if (!form.fileId) {
    ElMessage.warning('请选择合同')
    return
  }
  saving.value = true
  try {
    await api.signCreate(form.fileId, form.provider)
    ElMessage.success('签署任务已发起')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function poll(row: SignTask) {
  const res = await api.signPoll(row.id)
  ElMessage.success(`状态：${statusLabel(res.data?.status || '')}`)
  await load()
}

function statusLabel(s: string) {
  const m: Record<string, string> = { created: '已创建', sent: '已送达', signed: '已签署', failed: '失败' }
  return m[s] || s
}
function fmtTime(t?: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

onMounted(load)
</script>

<style scoped>
.toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.card-title { font-size: 16px; font-weight: 600; margin-right: 12px; }
.card-sub { font-size: 12px; color: #909399; }
.err { color: #f56c6c; font-size: 12px; }
.dim { color: #c0c4cc; }
</style>
