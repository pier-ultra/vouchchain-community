<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">合同模板库</span>
        <span class="card-sub">A6：模板管理（{{变量}} 占位符）+ 变量套打，与 AI 生成互补（法大大/上上签核心收费点）</span>
      </div>
      <div>
        <el-input v-model="query.keyword" placeholder="模板名/正文" clearable style="width: 200px" @keyup.enter="load" @clear="load" />
        <el-button type="primary" plain @click="load">查询</el-button>
        <el-button type="primary" @click="openEdit()">新建模板</el-button>
      </div>
    </div>

    <el-table :data="rows" v-loading="loading" border stripe size="small">
      <el-table-column prop="id" label="ID" width="56" />
      <el-table-column prop="name" label="模板名称" min-width="160" show-overflow-tooltip />
      <el-table-column prop="category" label="适用类型" width="110" />
      <el-table-column label="变量数" width="90">
        <template #default="{ row }">{{ (row.varKeys || []).length }}</template>
      </el-table-column>
      <el-table-column prop="remark" label="备注" min-width="140" show-overflow-tooltip />
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button link size="small" type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link size="small" type="warning" @click="openRender(row)">套打</el-button>
          <el-button link size="small" type="danger" @click="del(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新建/编辑 -->
    <el-dialog v-model="editVisible" :title="form.id ? '编辑模板' : '新建模板'" width="640px">
      <el-form label-width="80px">
        <el-form-item label="模板名称" required>
          <el-input v-model="form.name" placeholder="如：采购合同（标准版）" />
        </el-form-item>
        <el-form-item label="适用类型">
          <el-input v-model="form.category" placeholder="如：采购/销售/服务/租赁/保密" />
        </el-form-item>
        <el-form-item label="模板正文" required>
          <el-input v-model="form.content" type="textarea" :rows="10"
            placeholder="正文用 {{变量}} 占位符，如：甲方{{甲方}}与乙方{{乙方}}就{{标的}}签订本合同，金额人民币{{金额}}元。" />
        </el-form-item>
        <el-form-item label="变量清单">
          <div style="width:100%">
            <el-tag v-for="k in detectedVars" :key="k" class="var-tag" size="small">{{ k }}</el-tag>
            <span v-if="!detectedVars.length" class="var-hint">正文中未检测到 {{变量}}，或直接编辑下方声明</span>
          </div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="可选" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 变量套打 -->
    <el-dialog v-model="renderVisible" :title="'套打：' + (renderRow?.name || '')" width="680px">
      <el-form label-width="90px" v-if="renderRow">
        <el-form-item v-for="v in renderVars" :key="v.key" :label="v.label || v.key">
          <el-input v-model="renderValues[v.key]" :placeholder="v.default ? '默认：' + v.default : '必填'" />
        </el-form-item>
        <el-form-item label="预览">
          <el-input :model-value="renderPreview" type="textarea" :rows="12" readonly />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renderVisible = false">关闭</el-button>
        <el-button type="primary" :loading="rendering" @click="doRender">套打生成</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, type ContractTemplateRow, type ContractTemplateVar } from '../api'

const rows = ref<ContractTemplateRow[]>([])
const loading = ref(false)
const query = reactive({ keyword: '' })

const editVisible = ref(false)
const saving = ref(false)
const form = reactive<{ id: number; name: string; category: string; content: string; remark: string; variables: ContractTemplateVar[] }>({
  id: 0, name: '', category: '', content: '', remark: '', variables: [],
})
const detectedVars = computed(() => {
  const re = /\{\{\s*([A-Za-z0-9_\u4e00-\u9fa5]+)\s*\}\}/g
  const seen: string[] = []
  let m: RegExpExecArray | null
  while ((m = re.exec(form.content))) {
    if (!seen.includes(m[1])) seen.push(m[1])
  }
  return seen
})

const renderVisible = ref(false)
const renderRow = ref<ContractTemplateRow | null>(null)
const renderValues = reactive<Record<string, string>>({})
const renderPreview = ref('')
const rendering = ref(false)
const renderVars = computed<ContractTemplateVar[]>(() => {
  const row = renderRow.value
  if (!row) return []
  const vars = row.variables?.length ? row.variables : (row.varKeys || []).map((k) => ({ key: k, label: k, required: true, default: '' }))
  return vars
})

async function load() {
  loading.value = true
  try {
    const res = await api.contractTemplates({ keyword: query.keyword || undefined })
    rows.value = res.data || []
  } finally {
    loading.value = false
  }
}

function openEdit(row?: ContractTemplateRow) {
  form.id = row?.id || 0
  form.name = row?.name || ''
  form.category = row?.category || ''
  form.content = row?.content || ''
  form.remark = row?.remark || ''
  form.variables = row?.variables ? JSON.parse(JSON.stringify(row.variables)) : []
  editVisible.value = true
}

async function save() {
  if (!form.name || !form.content) {
    ElMessage.warning('模板名称与正文必填')
    return
  }
  saving.value = true
  try {
    const vars = form.variables.length ? form.variables : detectedVars.value.map((k) => ({ key: k, label: k, required: true, default: '' }))
    const d = { name: form.name, category: form.category, content: form.content, remark: form.remark, variables: vars }
    if (form.id) {
      await api.contractTemplateUpdate(form.id, d)
      ElMessage.success('已更新')
    } else {
      await api.contractTemplateCreate(d)
      ElMessage.success('已新建')
    }
    editVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function del(row: ContractTemplateRow) {
  await ElMessageBox.confirm(`删除模板「${row.name}」？`, '确认', { type: 'warning' })
  await api.contractTemplateDelete(row.id)
  ElMessage.success('已删除')
  await load()
}

function openRender(row: ContractTemplateRow) {
  renderRow.value = row
  Object.keys(renderValues).forEach((k) => delete renderValues[k])
  renderVars.value.forEach((v) => (renderValues[v.key] = v.default || ''))
  renderPreview.value = row.content
  renderVisible.value = true
}

async function doRender() {
  if (!renderRow.value) return
  rendering.value = true
  try {
    const res = await api.contractTemplateRender(renderRow.value.id, { ...renderValues })
    renderPreview.value = res.data?.content || ''
    if (res.data?.missing?.length) {
      ElMessage.warning('缺失必填变量：' + res.data.missing.join('、'))
    } else {
      ElMessage.success('套打完成，可复制/导出')
    }
  } finally {
    rendering.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.var-tag {
  margin-right: 6px;
}
.var-hint {
  color: var(--ch-text-soft, #909399);
  font-size: 12px;
}
</style>
