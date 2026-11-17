<template>
  <div>
    <el-card shadow="never">
      <div class="toolbar-line">
        <span class="hint">技能包 = zip 内含 SKILL.md（frontmatter 声明 name/description/triggers，正文为注入 AI 的指令）。磁盘保存为权威源（升级不丢），zip 原件同步登记进文件中心备份（local=本机 / s3=SeaweedFS）。</span>
        <div style="flex: 1" />
        <el-upload :show-file-list="false" accept=".zip" :auto-upload="false" :on-change="onPick">
          <el-button type="primary" :loading="uploading">上传技能包</el-button>
        </el-upload>
      </div>
      <el-table :data="rows" v-loading="loading" size="small">
        <el-table-column prop="id" label="ID" width="56" />
        <el-table-column label="技能" min-width="200">
          <template #default="{ row }">
            <b>{{ row.displayName || row.name }}</b>
            <div class="desc">{{ row.description }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="triggers" label="触发词" min-width="160" show-overflow-tooltip />
        <el-table-column prop="version" label="版本" width="70" />
        <el-table-column prop="path" label="磁盘位置" min-width="130" show-overflow-tooltip />
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="(v: boolean) => toggle(row, v)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80" fixed="right">
          <template #default="{ row }">
            <el-button link type="danger" size="small" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { UploadFile } from 'element-plus'
import { api } from '../api'

const rows = ref<any[]>([])
const loading = ref(false)
const uploading = ref(false)

async function load() {
  loading.value = true
  try {
    const res = await api.http.get<never, any>('/skills')
    rows.value = res.data || []
  } finally {
    loading.value = false
  }
}

async function onPick(f: UploadFile) {
  if (!f.raw) return
  uploading.value = true
  const fd = new FormData()
  fd.append('file', f.raw)
  try {
    const res = await api.http.post<never, any>('/skills', fd)
    ElMessage.success(`技能「${res.data?.name}」已安装（备份登记文件中心 #${res.data?.fileId}）`)
    load()
  } finally {
    uploading.value = false
  }
}

async function toggle(row: any, v: boolean) {
  await api.http.put<never, any>(`/skills/${row.id}/toggle`, { enabled: v })
  ElMessage.success(v ? '已启用（对话自动匹配触发词）' : '已停用')
  load()
}

async function del(row: any) {
  await api.http.delete<never, any>(`/skills/${row.id}`)
  ElMessage.success('已删除（磁盘目录与备份一并清理）')
  load()
}

onMounted(load)
</script>

<style scoped>
.toolbar-line { display: flex; gap: 10px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.hint { font-size: 12px; color: #909399; max-width: 720px; line-height: 1.6; }
.desc { font-size: 12px; color: #909399; margin-top: 2px; }
</style>
