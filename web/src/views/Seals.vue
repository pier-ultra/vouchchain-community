<template>
  <div>
    <el-tabs v-model="tab" type="border-card">
      <!-- 印章台账 -->
      <el-tab-pane label="印章台账" name="seals">
        <div class="toolbar-line">
          <el-input v-model="newSeal.name" placeholder="印章名称" style="width: 180px" />
          <el-select v-model="newSeal.type" style="width: 130px">
            <el-option label="合同专用章" value="contract" />
            <el-option label="公章" value="official" />
            <el-option label="财务章" value="finance" />
            <el-option label="法人章" value="legal" />
            <el-option label="其他" value="other" />
          </el-select>
          <el-input v-model="newSeal.keeper" placeholder="保管人" style="width: 120px" />
          <el-button type="primary" @click="addSeal">登记印章</el-button>
        </div>
        <el-table :data="seals" size="small" border v-loading="loading">
          <el-table-column prop="id" label="ID" width="56" />
          <el-table-column prop="name" label="印章" width="180" />
          <el-table-column label="类型" width="110">
            <template #default="{ row }">{{ typeLabel(row.type) }}</template>
          </el-table-column>
          <el-table-column prop="keeper" label="保管人" width="100" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150" fixed="right">
            <template #default="{ row }">
              <el-button v-if="row.status === 'active'" link type="warning" size="small" @click="setStatus(row, 'disabled')">停用</el-button>
              <el-button v-if="row.status === 'disabled'" link type="success" size="small" @click="setStatus(row, 'active')">恢复</el-button>
              <el-button v-if="row.status !== 'destroyed'" link type="danger" size="small" @click="setStatus(row, 'destroyed')">销毁</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 用印申请与台账 -->
      <el-tab-pane label="用印申请与台账" name="apply">
        <div class="toolbar-line">
          <el-select v-model="apply.sealId" placeholder="印章" style="width: 170px">
            <el-option v-for="s in seals.filter(x => x.status === 'active')" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
          <el-input-number v-model="apply.bizId" :min="0" placeholder="业务对象ID(合同文件)" style="width: 170px" />
          <el-input-number v-model="apply.fileId" :min="0" placeholder="文件中心ID" style="width: 140px" />
          <el-input-number v-model="apply.copies" :min="1" placeholder="份数" style="width: 100px" />
          <el-input v-model="apply.reason" placeholder="用印事由" style="width: 200px" />
          <el-button type="primary" :loading="applying" @click="addApply">提交申请</el-button>
        </div>
        <el-alert v-for="(w, i) in applyWarnings" :key="i" :title="w" type="warning" :closable="true" style="margin-bottom: 6px" />
        <el-table :data="applies" size="small" border v-loading="applyLoading">
          <el-table-column prop="id" label="单号" width="60" />
          <el-table-column label="印章" width="140">
            <template #default="{ row }">{{ sealName(row.seal_id) }}</template>
          </el-table-column>
          <el-table-column prop="reason" label="事由" min-width="160" show-overflow-tooltip />
          <el-table-column prop="copies" label="份数" width="60" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="approver" label="审批人" width="90" />
          <el-table-column prop="operator" label="操作人" width="90" />
          <el-table-column prop="stamped_at" label="用印时间" width="150" />
          <el-table-column prop="es_file_no" label="电子章编号" width="140" show-overflow-tooltip />
          <el-table-column label="操作" width="210" fixed="right">
            <template #default="{ row }">
              <el-button v-if="row.status === 'pending' && role !== 'viewer'" link type="success" size="small" @click="approve(row, true)">批准</el-button>
              <el-button v-if="row.status === 'pending' && role !== 'viewer'" link type="danger" size="small" @click="approve(row, false)">驳回</el-button>
              <el-button v-if="row.status === 'approved'" link type="primary" size="small" @click="stamp(row)">登记用印</el-button>
              <el-button v-if="row.status === 'stamped' && role !== 'viewer'" link size="small" @click="esStamp(row)">回填电子章</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 风险预警 -->
      <el-tab-pane label="风险预警" name="risk">
        <el-button :loading="scanning" @click="scan">扫描</el-button>
        <div style="margin-top: 12px">
          <el-alert v-for="(w, i) in risks" :key="i" :title="w" type="warning" :closable="false" style="margin-bottom: 6px" />
          <el-empty v-if="scanned && risks.length === 0" description="未发现异常用印" :image-size="60" />
        </div>
      </el-tab-pane>

      <!-- R-B11 外部电子签平台配置与盖章记录回填 -->
      <el-tab-pane label="电子签回填" name="es">
        <el-alert type="info" :closable="false" style="margin-bottom: 10px"
          title="电子真章不做（CA 资质墙）；这里只对接外部电子签平台，把盖章记录回填用印台账。未配置平台地址或平台失败时明确报错，不会谎报「已同步」。" />
        <div class="toolbar-line">
          <span>平台标识</span>
          <el-input v-model="es.platform" placeholder="如 esign-ali" style="width: 150px" />
          <span>盖章记录查询地址</span>
          <el-input v-model="es.endpoint" placeholder="https://…/stamp-record/query" style="width: 320px" />
          <span>凭据</span>
          <el-input v-model="es.token" placeholder="留空=不改" show-password style="width: 150px" />
          <el-switch v-model="esEnabled" active-value="on" inactive-value="off" active-text="启用回填" />
          <el-input v-model="es.remark" placeholder="备注" style="width: 140px" />
          <el-button v-if="role !== 'viewer'" type="primary" :loading="esSaving" @click="saveES">保存配置</el-button>
          <el-button @click="getESConfig">刷新</el-button>
        </div>
        <div class="toolbar-line" style="margin-top: 10px">
          <el-button :loading="esRecLoading" @click="loadESRecords">刷新回填记录</el-button>
          <span class="hint">回填入口在「用印申请与台账」每行操作列（已用印单）；每次回填留痕一条记录。</span>
        </div>
        <el-table :data="esRecords" size="small" border v-loading="esRecLoading">
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column prop="apply_id" label="用印单号" width="90" />
          <el-table-column prop="platform" label="平台" width="110" />
          <el-table-column prop="ext_ref" label="外部流水号" width="140" show-overflow-tooltip />
          <el-table-column prop="es_file_no" label="电子章编号" width="150" show-overflow-tooltip />
          <el-table-column prop="signer" label="签署人" width="100" />
          <el-table-column prop="signed_at" label="平台盖章时间" width="150" />
          <el-table-column prop="creator" label="回填人" width="90" />
          <el-table-column prop="created_at" label="回填时间" width="150" />
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'

const role = localStorage.getItem('ch_role') || 'viewer'
const tab = ref('seals')
const seals = ref<any[]>([])
const applies = ref<any[]>([])
const loading = ref(false)
const applyLoading = ref(false)
const applying = ref(false)
const scanning = ref(false)
const scanned = ref(false)
const risks = ref<string[]>([])
const applyWarnings = ref<string[]>([])
const newSeal = ref({ name: '', type: 'contract', keeper: '' })
const apply = ref({ sealId: 0, bizId: 0, fileId: 0, copies: 1, reason: '' })

function typeLabel(t: string) {
  return ({ contract: '合同专用章', official: '公章', finance: '财务章', legal: '法人章', other: '其他' } as any)[t] || t
}
function statusLabel(s: string) {
  return ({ active: '在用', disabled: '停用', destroyed: '已销毁', pending: '待审批', approved: '已批准', stamped: '已用印', rejected: '已驳回' } as any)[s] || s
}
function statusType(s: string) {
  return ({ pending: 'warning', approved: 'primary', stamped: 'success', rejected: 'danger' } as any)[s] || 'info'
}
function sealName(id: number) {
  return seals.value.find(s => s.id === id)?.name || ('#' + id)
}

async function load() {
  loading.value = true
  applyLoading.value = true
  try {
    const r1 = await api.http.get<never, any>('/seals')
    seals.value = r1.data || []
    const r2 = await api.http.get<never, any>('/seal-applies')
    applies.value = r2.data || []
  } finally {
    loading.value = false
    applyLoading.value = false
  }
}

async function addSeal() {
  await api.http.post<never, any>('/seals', newSeal.value)
  ElMessage.success('印章已登记')
  load()
}

async function setStatus(row: any, status: string) {
  if (status === 'destroyed') {
    try { await ElMessageBox.confirm(`确认登记印章「${row.name}」销毁？`, '销毁', { type: 'warning' }) } catch { return }
  }
  await api.http.put<never, any>(`/seals/${row.id}/status`, { status })
  ElMessage.success('状态已更新')
  load()
}

async function addApply() {
  applying.value = true
  applyWarnings.value = []
  try {
    const res = await api.http.post<never, any>('/seal-applies', apply.value)
    applyWarnings.value = res.data?.warnings || []
    if (applyWarnings.value.length) ElMessage.warning('已提交，但有风险提示')
    else ElMessage.success('申请已提交，待审批')
    load()
  } finally {
    applying.value = false
  }
}

async function approve(row: any, ok: boolean) {
  let comment = ''
  if (!ok) {
    try { comment = await ElMessageBox.prompt('驳回意见', '驳回用印申请').then(r => r.value) } catch { return }
  }
  await api.http.post<never, any>(`/seal-applies/${row.id}/approve`, { approve: ok, comment })
  ElMessage.success(ok ? '已批准' : '已驳回')
  load()
}

async function stamp(row: any) {
  await api.http.post<never, any>(`/seal-applies/${row.id}/stamp`, {})
  ElMessage.success('用印已登记（七字段留痕）')
  load()
}

async function scan() {
  scanning.value = true
  try {
    const res = await api.http.get<never, any>('/seal-risks')
    risks.value = res.data || []
    scanned.value = true
  } finally {
    scanning.value = false
  }
}

// ---------- R-B11 外部电子签平台配置与盖章记录回填 ----------
const es = ref({ platform: '', endpoint: '', token: '', enabled: 'off', remark: '' })
const esEnabled = ref('off')
const esSaving = ref(false)
const esRecLoading = ref(false)
const esRecords = ref<any[]>([])

async function getESConfig() {
  const r: any = await api.http.get<never, any>('/es/config')
  if (r.code !== 0) { ElMessage.error(r.message || '读取电子签配置失败'); return }
  Object.assign(es.value, r.data || {})
  esEnabled.value = es.value.enabled || 'off'
}

async function saveES() {
  esSaving.value = true
  try {
    const r: any = await api.http.post<never, any>('/es/config', { ...es.value, enabled: esEnabled.value })
    if (r.code !== 0) { ElMessage.error(r.message || '保存失败'); return }
    ElMessage.success('电子签配置已保存')
    await getESConfig()
  } finally { esSaving.value = false }
}

// 按用印申请回填外部平台盖章记录（未配置平台地址时后端明确报错）
async function esStamp(row: any) {
  const r: any = await api.http.post<never, any>(`/seal-applies/${row.id}/es-stamp`)
  if (r.code !== 0) { ElMessage.error(r.message || '回填失败'); return }
  ElMessage.success('电子章记录已回填')
  await Promise.all([load(), loadESRecords()])
}

async function loadESRecords() {
  esRecLoading.value = true
  try {
    const r: any = await api.http.get<never, any>('/es/records')
    esRecords.value = r.data || []
  } finally { esRecLoading.value = false }
}

onMounted(() => {
  load()
  getESConfig().catch(() => {})
  loadESRecords().catch(() => {})
})
</script>

<style scoped>
.toolbar-line { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; flex-wrap: wrap; }
</style>
