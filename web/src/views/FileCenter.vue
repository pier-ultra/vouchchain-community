<template>
  <div>
    <el-card shadow="never">
      <div style="display: flex; gap: 10px; align-items: center; margin-bottom: 12px">
        <el-select v-model="bizType" style="width: 150px" placeholder="全部类型" clearable @change="load">
          <el-option label="合同" value="contract" />
          <el-option label="发票" value="invoice" />
          <el-option label="回单" value="receipt" />
          <el-option label="知识库" value="kb" />
          <el-option label="其他" value="other" />
        </el-select>
        <el-input v-model="keyword" style="width: 220px" placeholder="按文件名搜索" clearable @keyup.enter="load" />
        <el-button @click="load">搜索</el-button>
        <div style="flex: 1" />
        <el-upload :show-file-list="false" :http-request="doUpload" multiple>
          <el-button type="primary" :loading="uploading">上传文件</el-button>
        </el-upload>
        <el-tag size="small" effect="plain" style="margin-left: 8px">
          存储：{{ storeKind === 's3' ? 'SeaweedFS (S3)' : '本地磁盘' }}
        </el-tag>
      </div>

      <el-table :data="rows" v-loading="loading" size="small">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="filename" label="文件名" min-width="220" show-overflow-tooltip />
        <el-table-column label="业务" width="150">
          <template #default="{ row }">{{ row.bizType }} #{{ row.bizId }}</template>
        </el-table-column>
        <el-table-column label="大小" width="100">
          <template #default="{ row }">{{ fmtSize(row.sizeBytes) }}</template>
        </el-table-column>
        <el-table-column prop="storage" label="存储" width="80" />
        <el-table-column prop="uploader" label="上传人" width="90" />
        <el-table-column prop="createdAt" label="时间" width="160" />
        <el-table-column label="操作" width="170" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="preview(row)">预览</el-button>
            <el-button link type="primary" size="small" @click="download(row)">下载</el-button>
            <el-button v-if="role !== 'viewer'" link type="danger" size="small" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'

const rows = ref<any[]>([])
const loading = ref(false)
const uploading = ref(false)
const bizType = ref('')
const keyword = ref('')
const storeKind = ref('local')
const role = localStorage.getItem('ch_role') || 'viewer'

function fmtSize(n: number): string {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (bizType.value) params.set('bizType', bizType.value)
    if (keyword.value) params.set('keyword', keyword.value)
    const res = await api.http.get<never, any>(`/files?` + params.toString())
    rows.value = res.data || []
    if (rows.value.length) storeKind.value = rows.value[0].storage
  } finally {
    loading.value = false
  }
}

async function doUpload(opt: any) {
  uploading.value = true
  const fd = new FormData()
  fd.append('file', opt.file)
  if (bizType.value) fd.append('bizType', bizType.value)
  try {
    const res = await api.http.post<never, any>('/files', fd)
    if (res.data?.dedup) ElMessage.info('内容重复，已复用已有文件')
    else ElMessage.success('上传成功（' + (res.data?.storage || '') + '）')
    load()
  } finally {
    uploading.value = false
  }
}

function preview(row: any) {
  window.open(`/api/files/${row.id}/preview`, '_blank')
}
function download(row: any) {
  window.open(`/api/files/${row.id}/download`, '_blank')
}
async function del(row: any) {
  try {
    await ElMessageBox.confirm(`删除「${row.filename}」？存储对象将一并清除。`, '删除文件', { type: 'warning' })
  } catch {
    return
  }
  await api.http.delete<never, any>(`/files/${row.id}`)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>
