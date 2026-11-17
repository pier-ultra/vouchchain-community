<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">模型配置</span>
        <span class="card-sub">Core 共用内核：统一托管 LLM / OCR / 重排序 / Embedding / 向量库接入</span>
      </div>
      <el-button type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon>
        新增配置
      </el-button>
    </div>

    <el-alert
      v-if="hasBadKey"
      type="warning"
      :closable="false"
      show-icon
      title="密钥不可用：AI 能力已降级"
      description="库内存在加密密钥，但本次服务启动未带 APP_AES_KEY（或值已变更）。请带同一主密钥重启服务，或在下方重新录入密钥；在此之前 AI 相关功能会自动回退，不会静默假装成功。"
      style="margin-bottom: 12px"
    />

    <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="name" label="配置名" min-width="140" />
      <el-table-column label="类别" width="110">
        <template #default="{ row }">
          <el-tag :type="categoryMap[row.category]?.type || 'info'">
            {{ categoryMap[row.category]?.label || row.category }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="provider" label="提供方" width="140" />
      <el-table-column prop="baseUrl" label="Base URL" min-width="220" show-overflow-tooltip />
      <el-table-column prop="modelName" label="模型" min-width="140" />
      <el-table-column label="密钥" width="140">
        <template #default="{ row }">
          <el-tag v-if="!row.apiKey" type="info">未配置</el-tag>
          <el-tooltip v-else-if="row.keyUsable === false" content="当前进程未设置 APP_AES_KEY 或密钥不匹配，AI 能力不可用" placement="top">
            <el-tag type="danger">密钥不可用</el-tag>
          </el-tooltip>
          <el-tag v-else type="success">{{ row.apiKey }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="90">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'danger'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="170">
        <template #default="{ row }">{{ fmtTime(row.updatedAt) }}</template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="新增模型配置" width="560px">
      <el-form label-width="100px">
        <el-form-item label="配置名" required>
          <el-input v-model="form.name" placeholder="如：agnes-llm" />
        </el-form-item>
        <el-form-item label="类别" required>
          <el-select v-model="form.category" style="width: 220px">
            <el-option v-for="(v, k) in categoryMap" :key="k" :label="v.label" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="提供方">
          <el-select v-model="form.provider" allow-create filterable default-first-option placeholder="选择或输入自定义" style="width: 220px">
            <el-option label="OpenAI 兼容（硅基流动/Agnes/本地 llama.cpp）" value="openai-compatible" />
            <el-option label="Ollama（URL 填 http://host:11434/v1）" value="ollama" />
            <el-option label="本地自建（TEI/RapidOCR 等自托管 HTTP）" value="local" />
            <el-option label="百度智能云 OCR" value="baidu-paddle" />
          </el-select>
        </el-form-item>
        <el-form-item label="Base URL">
          <el-input v-model="form.baseUrl" placeholder="如：https://api.agnes-ai.cn/v1" />
        </el-form-item>
        <el-form-item label="模型名">
          <el-input v-model="form.modelName" placeholder="如：agnes-3.0-flash" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input
            v-model="form.apiKey"
            type="password"
            show-password
            placeholder="可选；填了则服务端加密存储，调用时自动解密"
          />
        </el-form-item>
        <el-form-item label="扩展参数">
          <el-input
            v-model="form.extraConfigText"
            type="textarea"
            :rows="3"
            placeholder='JSON，如 {"dim":768,"protocol":"openai-chat"}'
          />
          <div class="extra-hint">
            可选。embedding 填 <code>dim</code>（向量维度，如讯飞 768）；ocr 填 <code>protocol</code>（<code>openai-chat</code>=OpenAI 多模态 / <code>multipart</code>=自建）；各类均可填 <code>path</code> 覆盖端点路径。也可用环境变量 EMBED_DIM / OCR_PROTOCOL 等。
          </div>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <el-alert
        type="success"
        :closable="false"
        title="API Key 加密存储（AES-256-GCM）"
        description="密钥加密入库，接口列表只返回脱敏值；大模型调用时服务端解密使用，明文不落日志"
      />
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api, type ModelConfig } from '../api'

const list = ref<ModelConfig[]>([])
const hasBadKey = computed(() => list.value.some(c => !!c.apiKey && c.keyUsable === false))
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)

const form = reactive({
  name: '',
  category: 'llm',
  provider: '',
  baseUrl: '',
  modelName: '',
  apiKey: '',
  extraConfigText: '',
  enabled: true,
})

const categoryMap: Record<string, { label: string; type: 'primary' | 'success' | 'warning' | 'danger' | 'info' }> = {
  llm: { label: '大模型', type: 'primary' },
  ocr: { label: 'OCR', type: 'success' },
  rerank: { label: '重排序', type: 'warning' },
  embedding: { label: 'Embedding', type: 'danger' },
  vector: { label: '向量库', type: 'info' },
}

async function load() {
  loading.value = true
  try {
    const res = await api.modelConfigs()
    list.value = res.data || []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.name = ''
  form.category = 'llm'
  form.provider = ''
  form.baseUrl = ''
  form.modelName = ''
  form.apiKey = ''
  form.extraConfigText = ''
  form.enabled = true
  dialogVisible.value = true
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning('请填写配置名')
    return
  }
  let extraConfig: Record<string, unknown> | null = null
  if (form.extraConfigText.trim()) {
    try {
      extraConfig = JSON.parse(form.extraConfigText)
    } catch {
      ElMessage.warning('扩展参数不是合法 JSON，请检查')
      return
    }
  }
  saving.value = true
  try {
    await api.createModelConfig({
      name: form.name,
      category: form.category,
      provider: form.provider,
      baseUrl: form.baseUrl,
      modelName: form.modelName,
      apiKey: form.apiKey,
      extraConfig,
      enabled: form.enabled,
    })
    ElMessage.success('已创建')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function fmtTime(t?: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

onMounted(load)
</script>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
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
.extra-hint {
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
  margin-top: 4px;
}
.extra-hint code {
  background: #f4f4f5;
  padding: 0 4px;
  border-radius: 3px;
  color: #409eff;
}
</style>
