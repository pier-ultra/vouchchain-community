<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">MCP 客户端连接</span>
        <span class="card-sub">系统作为客户端连接外部 MCP server，把外部工具动态注册进 AI 对话</span>
      </div>
      <el-button type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon>
        新增连接
      </el-button>
    </div>

    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="接入说明"
      description="配置 endpoint + token 后，系统会连接外部 MCP server 并拉取 tools/list，把每个工具注册为「ext_连接ID_工具名」的 AI 对话工具（与内置 9 个工具同一张工具表）。启用即自动注册，停用/删除即移除。"
      style="margin-bottom: 12px"
    />

    <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="name" label="连接名" min-width="130" />
      <el-table-column prop="endpoint" label="Endpoint" min-width="240" show-overflow-tooltip />
      <el-table-column label="状态" width="120">
        <template #default="{ row }">
          <el-tag v-if="row.lastStatus === 'ok'" type="success">已连接</el-tag>
          <el-tag v-else-if="row.lastStatus === 'error'" type="danger">错误</el-tag>
          <el-tag v-else-if="row.lastStatus === 'pending'" type="warning">加载中</el-tag>
          <el-tag v-else type="info">未启用</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="工具数" width="90" align="center">
        <template #default="{ row }">{{ row.toolCount || 0 }}</template>
      </el-table-column>
      <el-table-column label="启用" width="90" align="center">
        <template #default="{ row }">
          <el-switch
            :model-value="row.enabled"
            :loading="togglingId === row.id"
            @change="(val: boolean) => toggleEnabled(row, val)"
          />
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="170">
        <template #default="{ row }">{{ fmtTime(row.updatedAt) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="success" :loading="testingId === row.id" @click="testRow(row)">测试</el-button>
          <el-button link type="warning" @click="showTools(row)">工具</el-button>
          <el-popconfirm title="确认删除该连接？" confirm-button-text="删除" cancel-button-text="取消" @confirm="remove(row)">
            <template #reference>
              <el-button link type="danger">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增/编辑 -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑 MCP 连接' : '新增 MCP 连接'" width="560px">
      <el-form label-width="110px">
        <el-form-item label="连接名" required>
          <el-input v-model="form.name" placeholder="如：cursor-server / 法务知识库" />
        </el-form-item>
        <el-form-item label="Endpoint" required>
          <el-input v-model="form.endpoint" placeholder="如：https://host/mcp（Streamable HTTP）" />
        </el-form-item>
        <el-form-item label="Bearer Token">
          <el-input
            v-model="form.token"
            type="password"
            show-password
            :placeholder="editingId ? '留空则不修改（当前为脱敏值）' : '可选；填了则服务端加密存储'"
          />
        </el-form-item>
        <el-form-item label="额外请求头">
          <el-input
            v-model="form.extraHeaders"
            type="textarea"
            :rows="2"
            placeholder='JSON，如 {"X-Org":"finance"}（可选，原样注入请求头）'
          />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" placeholder="可选说明" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <el-alert
        type="success"
        :closable="false"
        title="Token 加密存储（AES-256-GCM）"
        description="与模型 API Key 同一机制：加密入库，列表只返回脱敏值，调用时服务端解密"
      />
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="warning" :loading="testing" @click="testDraft">测试连接</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 测试结果 -->
    <el-dialog v-model="testResultVisible" title="MCP 连接测试" width="480px">
      <el-result
        :icon="testResult.ok ? 'success' : 'error'"
        :title="testResult.ok ? '连接成功' : '测试失败'"
        :sub-title="testResult.message"
      >
        <template v-if="testResult.detail" #extra>
          <div class="test-detail">{{ testResult.detail }}</div>
        </template>
      </el-result>
      <template #footer>
        <el-button type="primary" @click="testResultVisible = false">知道了</el-button>
      </template>
    </el-dialog>

    <!-- 工具预览 -->
    <el-dialog v-model="toolsVisible" :title="`工具列表 · ${toolsServerName || ''}`" width="640px">
      <el-table :data="tools" v-loading="toolsLoading" border max-height="420">
        <el-table-column prop="name" label="工具名" width="220" />
        <el-table-column label="参数">
          <template #default="{ row }">
            <span v-if="!row.required || !row.required.length">—</span>
            <el-tag v-for="r in row.required" :key="r" size="small" style="margin-right: 4px">{{ r }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="说明" show-overflow-tooltip />
      </el-table>
      <template #footer>
        <el-button type="primary" @click="toolsVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type McpServer, type McpToolInfo } from '../api'

const list = ref<McpServer[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const togglingId = ref<number | null>(null)
const testingId = ref<number | null>(null)
const testing = ref(false)
const testResultVisible = ref(false)
const testResult = reactive({ ok: false, message: '', detail: '' })
const toolsVisible = ref(false)
const toolsLoading = ref(false)
const tools = ref<McpToolInfo[]>([])
const toolsServerName = ref('')

const form = reactive({
  name: '',
  endpoint: '',
  token: '',
  extraHeaders: '',
  description: '',
  enabled: true,
})

async function load() {
  loading.value = true
  try {
    const res = await api.mcpServers()
    list.value = res.data || []
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.name = ''
  form.endpoint = ''
  form.token = ''
  form.extraHeaders = ''
  form.description = ''
  form.enabled = true
}

function openCreate() {
  editingId.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: McpServer) {
  editingId.value = row.id
  form.name = row.name
  form.endpoint = row.endpoint
  form.token = '' // 编辑时留空 = 不改 token
  form.extraHeaders = row.extraHeaders || ''
  form.description = row.description
  form.enabled = row.enabled
  dialogVisible.value = true
}

function validateExtraHeaders(): boolean {
  if (!form.extraHeaders.trim()) return true
  try {
    JSON.parse(form.extraHeaders)
    return true
  } catch {
    ElMessage.warning('额外请求头不是合法 JSON，请检查')
    return false
  }
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning('请填写连接名')
    return
  }
  if (!form.endpoint.trim()) {
    ElMessage.warning('请填写 Endpoint')
    return
  }
  if (!validateExtraHeaders()) return
  saving.value = true
  try {
    const payload = {
      name: form.name,
      endpoint: form.endpoint,
      token: form.token, // 编辑时留空 = 不改 token
      extraHeaders: form.extraHeaders || '{}',
      description: form.description,
      enabled: form.enabled,
    }
    if (editingId.value) {
      await api.updateMcpServer(editingId.value, payload)
      ElMessage.success('已更新')
    } else {
      await api.createMcpServer(payload)
      ElMessage.success('已创建')
    }
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(row: McpServer, val: boolean) {
  togglingId.value = row.id
  try {
    await api.toggleMcpServer(row.id, val)
    row.enabled = val
    ElMessage.success(val ? '已启用' : '已停用')
  } catch {
    await load()
  } finally {
    togglingId.value = null
  }
}

async function remove(row: McpServer) {
  await api.deleteMcpServer(row.id)
  ElMessage.success('已删除')
  await load()
}

function showTestResult(ok: boolean, message: string, detail = '') {
  testResult.ok = ok
  testResult.message = message
  testResult.detail = detail
  testResultVisible.value = true
}

// 行「测试」：测库里已保存的配置（后端解密 token 后实测）
async function testRow(row: McpServer) {
  testingId.value = row.id
  try {
    const res = await api.testMcpServer({ id: row.id })
    if (res.code === 0) {
      showTestResult(true, res.message || '连接成功', `拉取到 ${res.data?.tools ?? 0} 个工具`)
    } else {
      showTestResult(false, res.message || '测试失败')
    }
  } catch (e: any) {
    showTestResult(false, e?.message || '请求异常')
  } finally {
    testingId.value = null
  }
}

// 对话框「测试连接」：用表单草稿实测（不落库）
async function testDraft() {
  if (!form.endpoint.trim()) {
    ElMessage.warning('请先填写 Endpoint')
    return
  }
  if (!validateExtraHeaders()) return
  testing.value = true
  try {
    const res = await api.testMcpServer({
      endpoint: form.endpoint,
      token: form.token, // 草稿里是明文
      extraHeaders: form.extraHeaders || '{}',
    })
    if (res.code === 0) {
      showTestResult(true, res.message || '连接成功', `拉取到 ${res.data?.tools ?? 0} 个工具`)
    } else {
      showTestResult(false, res.message || '测试失败')
    }
  } catch (e: any) {
    showTestResult(false, e?.message || '请求异常')
  } finally {
    testing.value = false
  }
}

// 工具预览：拉取该连接已注册的工具
async function showTools(row: McpServer) {
  toolsServerName.value = row.name
  toolsVisible.value = true
  toolsLoading.value = true
  try {
    const res = await api.mcpServerTools(row.id)
    tools.value = res.data || []
  } finally {
    toolsLoading.value = false
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
.test-detail {
  font-size: 12px;
  color: #909399;
  word-break: break-all;
  text-align: left;
  background: #f4f4f5;
  padding: 8px 10px;
  border-radius: 4px;
}
</style>
