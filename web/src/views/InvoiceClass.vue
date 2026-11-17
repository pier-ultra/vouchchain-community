<template>
  <div>
    <!-- 统计条 -->
    <el-card shadow="never" style="margin-bottom: 12px">
      <div style="display: flex; gap: 10px; align-items: center; flex-wrap: wrap">
        <el-tag v-for="(cnt, cat) in stats" :key="cat" :type="cat === '未分类' ? 'info' : 'success'" effect="plain">
          {{ cat }}：{{ cnt }}
        </el-tag>
        <div style="flex: 1" />
        <el-checkbox v-model="onlyEmpty">只处理未分类</el-checkbox>
        <el-button type="primary" :loading="running" @click="run">执行分类（规则 + AI 兜底）</el-button>
      </div>
    </el-card>

    <!-- 规则列表 -->
    <el-card shadow="never">
      <div style="display: flex; justify-content: space-between; margin-bottom: 10px">
        <b>分类规则（按优先级从小到大匹配，未命中的走 AI 判断）</b>
        <el-button size="small" @click="openEdit()">新增规则</el-button>
      </div>
      <el-table :data="rules" v-loading="loading" size="small">
        <el-table-column prop="priority" label="优先级" width="80" />
        <el-table-column prop="name" label="规则名" width="140" />
        <el-table-column prop="category" label="分类" width="110">
          <template #default="{ row }"><el-tag size="small">{{ row.category }}</el-tag></template>
        </el-table-column>
        <el-table-column label="扫描字段" width="90">
          <template #default="{ row }">{{ fieldLabel(row.field) }}</template>
        </el-table-column>
        <el-table-column prop="keywords" label="关键词（逗号分隔）" min-width="260" show-overflow-tooltip />
        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="(v: any) => toggle(row, v)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" size="small" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 编辑弹窗 -->
    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑规则' : '新增规则'" width="520px">
      <el-form label-width="90px">
        <el-form-item label="规则名" required>
          <el-input v-model="form.name" placeholder="如：差旅交通" />
        </el-form-item>
        <el-form-item label="目标分类" required>
          <el-select v-model="form.category" allow-create filterable style="width: 100%">
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
        </el-form-item>
        <el-form-item label="扫描字段">
          <el-select v-model="form.field" style="width: 100%">
            <el-option label="全部字段（开票方+商品）" value="any" />
            <el-option label="仅开票方" value="seller" />
            <el-option label="仅商品/服务名" value="items" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词" required>
          <el-input v-model="form.keywords" type="textarea" :rows="2" placeholder="逗号分隔，如：机票,火车,酒店,住宿" />
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="form.priority" :min="1" :max="999" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const rules = ref<any[]>([])
const stats = ref<Record<string, number>>({})
const loading = ref(false)
const running = ref(false)
const onlyEmpty = ref(true)
const dialogVisible = ref(false)
const categories = ['设备采购', '咨询服务', '软件服务', '办公用品', '差旅交通', '餐饮招待', '广告宣传', '其他']

const form = ref<any>({ id: 0, name: '', category: '', field: 'any', keywords: '', priority: 100 })

function fieldLabel(f: string) {
  return f === 'seller' ? '开票方' : f === 'items' ? '商品名' : '全部字段'
}

async function load() {
  loading.value = true
  try {
    const r1 = await api.http.get<never, any>('/invoice-class/rules')
    rules.value = r1.data || []
    const r2 = await api.http.get<never, any>('/invoice-class/stats')
    stats.value = r2.data || {}
  } finally {
    loading.value = false
  }
}

function openEdit(row?: any) {
  form.value = row
    ? { ...row }
    : { id: 0, name: '', category: '', field: 'any', keywords: '', priority: (rules.value.length + 1) * 10 }
  dialogVisible.value = true
}

async function save() {
  await api.http.post<never, any>('/invoice-class/rules', form.value)
  ElMessage.success('已保存')
  dialogVisible.value = false
  load()
}

async function toggle(row: any, v: boolean) {
  await api.http.post<never, any>('/invoice-class/rules', { ...row, enabled: v })
  load()
}

async function del(row: any) {
  await api.http.delete<never, any>(`/invoice-class/rules/${row.id}`)
  ElMessage.success('已删除')
  load()
}

async function run() {
  running.value = true
  try {
    const res = await api.http.post<never, any>('/invoice-class/run', { onlyEmpty: onlyEmpty.value })
    const d = res.data || {}
    ElMessage.success(`完成：规则命中 ${d.ruleHit}，AI 判断 ${d.aiHit}，跳过 ${d.skipped}`)
    load()
  } finally {
    running.value = false
  }
}

onMounted(load)
</script>
