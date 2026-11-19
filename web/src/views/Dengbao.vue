<template>
  <div>
    <el-alert type="info" :closable="false" style="margin-bottom: 10px"
      title="等保证据链（企业侧）：平时攒证据，测评时拿得出来。不替代测评执行，只做留痕、留存与按测评口径导出。" />
    <el-tabs v-model="tab" type="border-card">
      <!-- R-C2 留存策略与到期归档（P0） -->
      <el-tab-pane label="留存策略与归档" name="retention">
        <el-card shadow="never" style="margin-bottom: 10px">
          <el-descriptions :column="4" size="small" border>
            <el-descriptions-item label="留存期限">{{ st.retentionDays }} 天</el-descriptions-item>
            <el-descriptions-item label="等保下限">{{ st.floorDays }} 天（6 个月）</el-descriptions-item>
            <el-descriptions-item label="期限达标">
              <el-tag :type="st.retentionOK ? 'success' : 'danger'" size="small">{{ st.retentionOK ? '达标' : '不达标' }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="最早日志">{{ st.earliestAt || '无' }}</el-descriptions-item>
            <el-descriptions-item label="证据链覆盖">{{ Number(st.coverageDays || 0).toFixed(1) }} 天</el-descriptions-item>
            <el-descriptions-item label="证据链连续">
              <el-tag :type="st.coverageOK ? 'success' : 'warning'" size="small">{{ st.coverageOK ? '连续' : '已断开' }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="在库日志">{{ st.liveCount }} 条</el-descriptions-item>
            <el-descriptions-item label="已归档">{{ st.archiveCount }} 条</el-descriptions-item>
          </el-descriptions>
        </el-card>
        <div class="toolbar-line">
          <span>留存期限</span>
          <el-input-number v-model="policy.retentionDays" :min="0" :max="3650" style="width: 130px" />
          <span>拟保级别</span>
          <el-select v-model="policy.level" style="width: 110px">
            <el-option label="二级" value="2" />
            <el-option label="一级" value="1" />
            <el-option label="三级" value="3" />
          </el-select>
          <el-button v-if="role !== 'viewer'" type="primary" :loading="saving" @click="savePolicy">保存策略</el-button>
          <el-button @click="loadStatus">刷新诊断</el-button>
          <el-button :loading="archiving" @click="archive(true)">试算归档</el-button>
          <el-button v-if="role !== 'viewer'" type="warning" :disabled="archiving" @click="archive(false)">执行到期归档</el-button>
          <span class="hint">归档是把到期日志搬到归档表（不是删除），总留存期不变；低于等保下限的配置会被后端拒绝。</span>
        </div>
        <el-alert v-if="archiveMsg" :title="archiveMsg" type="success" :closable="true" style="margin-top: 8px" />
      </el-tab-pane>

      <!-- R-C4 访问审计导出（测评格式） -->
      <el-tab-pane label="访问审计取证" name="evidence">
        <el-table :data="blocks" size="small" border v-loading="evLoading" style="max-width: 620px">
          <el-table-column prop="block" label="证据块" width="160" />
          <el-table-column prop="count" label="条数" width="120" />
          <el-table-column label="操作" width="180">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="downloadEvidence(row.block)">导出该块 Excel</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="toolbar-line" style="margin-top: 10px">
          <el-input v-model="username" placeholder="按用户过滤" clearable style="width: 150px" />
          <el-input v-model="since" placeholder="起始日期 YYYY-MM-DD" clearable style="width: 170px" />
          <el-button :loading="evLoading" @click="loadBlocks">重算条数</el-button>
          <el-button type="success" :loading="packing" @click="downloadPack">导出证据包（zip + 清单）</el-button>
          <span class="hint">三块口径：身份鉴别 / 访问控制 / 安全审计（全量）。证据包内含三块 + 权限变更链 + 资产 + 整改 + 备份 + 清单。</span>
        </div>
      </el-tab-pane>

      <!-- R-C3 权限变更证据链（P0） -->
      <el-tab-pane label="权限变更证据链" name="perm">
        <div class="toolbar-line">
          <el-input v-model="username" placeholder="按用户过滤" clearable style="width: 150px" />
          <el-input v-model="since" placeholder="起始日期 YYYY-MM-DD" clearable style="width: 170px" />
          <el-button :loading="permLoading" @click="loadPermChain">查询</el-button>
          <el-button type="primary" @click="downloadPermChain">导出 Excel（等保口径）</el-button>
          <span class="hint">角色授权 / 用户绑定 / SCIM 账户处置的动作留痕，按时间正序=证据链顺序。</span>
        </div>
        <el-table :data="permChain" size="small" border v-loading="permLoading">
          <el-table-column prop="scope" label="来源" width="80">
            <template #default="{ row }">{{ row.scope === 'archive' ? '已归档' : '在库' }}</template>
          </el-table-column>
          <el-table-column prop="created_at" label="时间" width="150" />
          <el-table-column prop="username" label="操作人" width="110" />
          <el-table-column prop="action" label="动作" width="170" />
          <el-table-column prop="target_type" label="对象类型" width="130" />
          <el-table-column prop="target_id" label="对象ID" width="80" />
          <el-table-column prop="detail" label="变更内容" min-width="220" show-overflow-tooltip />
          <el-table-column prop="ip" label="来源IP" width="110" />
        </el-table>
      </el-tab-pane>

      <!-- R-C1 等保资产台账 -->
      <el-tab-pane label="等保资产台账" name="assets">
        <div class="toolbar-line">
          <el-input v-model="asset.name" placeholder="系统/设备名称" style="width: 180px" />
          <el-select v-model="asset.category" style="width: 130px">
            <el-option label="应用" value="application" />
            <el-option label="系统" value="system" />
            <el-option label="设备" value="device" />
          </el-select>
          <el-select v-model="asset.level" style="width: 110px">
            <el-option label="二级" value="2" />
            <el-option label="一级" value="1" />
            <el-option label="三级" value="3" />
          </el-select>
          <el-select v-model="asset.critical" style="width: 110px">
            <el-option label="重要" value="critical" />
            <el-option label="一般" value="normal" />
          </el-select>
          <el-input v-model="asset.owner" placeholder="责任人" style="width: 110px" />
          <el-input v-model="asset.department" placeholder="部门" style="width: 110px" />
          <el-input v-model="asset.addr" placeholder="部署地址/网段" style="width: 150px" />
          <el-button v-if="role !== 'viewer'" type="primary" :loading="assetSaving" @click="saveAsset">登记资产</el-button>
          <span class="hint">共 {{ assetTotal }} 项；只登记不纳管。</span>
        </div>
        <el-table :data="assets" size="small" border v-loading="assetLoading">
          <el-table-column prop="id" label="ID" width="56" />
          <el-table-column prop="name" label="资产" width="180" />
          <el-table-column label="类别" width="90">
            <template #default="{ row }">{{ categoryLabel(row.category) }}</template>
          </el-table-column>
          <el-table-column prop="level" label="拟保级别" width="90" />
          <el-table-column label="重要程度" width="90">
            <template #default="{ row }">{{ row.critical === 'critical' ? '重要' : '一般' }}</template>
          </el-table-column>
          <el-table-column prop="owner" label="责任人" width="100" />
          <el-table-column prop="department" label="部门" width="100" />
          <el-table-column prop="addr" label="部署地址" width="140" show-overflow-tooltip />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">{{ row.status === 'active' ? '在用' : '下线' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <el-button v-if="role !== 'viewer'" link :type="row.status === 'active' ? 'warning' : 'success'" size="small"
                @click="toggleAsset(row)">{{ row.status === 'active' ? '下线' : '恢复在用' }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- R-C5 整改跟踪 -->
      <el-tab-pane label="整改跟踪" name="findings">
        <div class="toolbar-line">
          <el-input v-model="finding.item" placeholder="控制点/测评项，如 安全审计-8.1.4.1" style="width: 220px" />
          <el-input v-model="finding.description" placeholder="问题描述" style="width: 220px" />
          <el-select v-model="finding.level" style="width: 110px">
            <el-option label="高" value="high" />
            <el-option label="中" value="medium" />
            <el-option label="低" value="low" />
          </el-select>
          <el-input v-model="finding.owner" placeholder="整改责任人" style="width: 120px" />
          <el-date-picker v-model="finding.dueDate" type="date" value-format="YYYY-MM-DD" placeholder="整改期限" style="width: 140px" />
          <el-select v-model="finding.assetId" placeholder="关联资产" clearable style="width: 150px">
            <el-option v-for="a in assets" :key="a.id" :label="a.name" :value="a.id" />
          </el-select>
          <el-button v-if="role !== 'viewer'" type="primary" :loading="findingSaving" @click="createFinding">登记整改项</el-button>
          <span class="hint">未闭环 {{ findingOpen }} 项。</span>
        </div>
        <el-table :data="findings" size="small" border v-loading="findingLoading">
          <el-table-column prop="id" label="ID" width="56" />
          <el-table-column prop="item" label="控制点/测评项" width="190" show-overflow-tooltip />
          <el-table-column label="级别" width="70">
            <template #default="{ row }">
              <el-tag :type="levelTag(row.level)" size="small">{{ levelLabel(row.level) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="description" label="问题描述" min-width="200" show-overflow-tooltip />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusTag(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="owner" label="整改人" width="100" />
          <el-table-column prop="verifier" label="复测人" width="100" />
          <el-table-column prop="due_date" label="整改期限" width="110" />
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <template v-if="role !== 'viewer'">
                <el-button v-if="row.status !== 'closed' && row.status !== 'verified'" link type="success" size="small" @click="verifyFinding(row)">复测通过</el-button>
                <el-button v-if="row.status === 'verified'" link type="warning" size="small" @click="closeFinding(row)">关闭</el-button>
                <el-button v-if="row.status !== 'open'" link type="danger" size="small" @click="reopenFinding(row)">退回整改</el-button>
              </template>
              <el-button link type="primary" size="small" @click="showLog(row)">时间线</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-dialog v-model="logVisible" title="整改时间线" width="620px">
          <el-table :data="findingLogs" size="small" border max-height="360">
            <el-table-column prop="created_at" label="时间" width="150" />
            <el-table-column prop="action" label="动作" width="100" />
            <el-table-column prop="actor" label="操作人" width="110" />
            <el-table-column prop="note" label="说明" min-width="180" show-overflow-tooltip />
          </el-table>
        </el-dialog>
      </el-tab-pane>

      <!-- R-C8 备份记录证据 -->
      <el-tab-pane label="备份记录" name="backups">
        <div class="toolbar-line">
          <el-select v-model="backup.source" style="width: 120px">
            <el-option label="数据库" value="database" />
            <el-option label="应用" value="app" />
            <el-option label="文件" value="file" />
          </el-select>
          <el-input v-model="backup.tool" placeholder="工具，如 pg_dump" style="width: 130px" />
          <el-input v-model="backup.fileName" placeholder="备份文件名" style="width: 190px" />
          <el-input v-model.number="backup.sizeBytes" placeholder="字节数" style="width: 100px" />
          <el-input v-model="backup.checksum" placeholder="sha256" style="width: 170px" />
          <el-select v-model="backup.status" style="width: 110px">
            <el-option label="成功" value="success" />
            <el-option label="失败" value="failed" />
            <el-option label="部分" value="partial" />
          </el-select>
          <el-button v-if="role !== 'viewer'" type="primary" :loading="backupSaving" @click="addBackup">登记备份</el-button>
          <span class="hint">窗口 {{ backupWindowDays }} 天：{{ backupSummary }}；最近成功 {{ backupLastSuccess || '无' }}。</span>
        </div>
        <el-table :data="backups" size="small" border v-loading="backupLoading">
          <el-table-column prop="id" label="ID" width="56" />
          <el-table-column label="来源" width="90">
            <template #default="{ row }">{{ sourceLabel(row.source) }}</template>
          </el-table-column>
          <el-table-column prop="tool" label="工具" width="110" />
          <el-table-column prop="file_name" label="备份文件" width="200" show-overflow-tooltip />
          <el-table-column prop="size_bytes" label="大小(字节)" width="110" />
          <el-table-column prop="checksum" label="校验和" width="150" show-overflow-tooltip />
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="started_at" label="开始" width="150" />
          <el-table-column prop="finished_at" label="完成" width="150" />
          <el-table-column prop="note" label="备注" min-width="140" show-overflow-tooltip />
        </el-table>
      </el-tab-pane>

    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'

const role = localStorage.getItem('ch_role') || 'viewer'
const tab = ref('retention')

// ---------- R-C2 留存策略与归档 ----------
const st = ref<any>({})
const policy = reactive({ retentionDays: 180, level: '2' })
const saving = ref(false)
const archiving = ref(false)
const archiveMsg = ref('')

async function loadStatus() {
  const p = await call(api.http.get<never, any>('/dengbao/policy'))
  if (p) {
    policy.retentionDays = p.retentionDays ?? policy.retentionDays
    policy.level = p.level || '2'
  }
  st.value = (await call(api.http.get<never, any>('/dengbao/retention/status'))) || {}
}

async function savePolicy() {
  saving.value = true
  try {
    await call(api.http.put<never, any>('/dengbao/policy', {
      retentionDays: policy.retentionDays, level: policy.level,
    }))
    ElMessage.success('留存策略已保存')
    await loadStatus()
  } finally { saving.value = false }
}

async function archive(dryRun: boolean) {
  archiving.value = true
  archiveMsg.value = ''
  try {
    const res = await call(api.http.post<never, any>('/dengbao/retention/archive', { dryRun }))
    archiveMsg.value = dryRun
      ? `试算：${res.moved} 条日志早于截止点 ${res.cutoff}（未执行）`
      : `已归档 ${res.moved} 条（截止点 ${res.cutoff}，搬到归档表，未删除）`
    if (!dryRun) ElMessage.success('到期归档完成')
    await loadStatus()
  } finally { archiving.value = false }
}

// ---------- R-C4 证据块导出 / R-C9 证据包 ----------
const blocks = ref<any[]>([])
const evLoading = ref(false)
const packing = ref(false)
const username = ref('')
const since = ref('')

async function loadBlocks() {
  evLoading.value = true
  try {
    blocks.value = (await call(api.http.get<never, any>('/dengbao/evidence/blocks', { params: { username: username.value, since: since.value } }))) || []
  } finally { evLoading.value = false }
}

async function downloadEvidence(block: string) {
  await downloadFile(
    `/dengbao/evidence/export?block=${encodeURIComponent(block)}&since=${encodeURIComponent(since.value)}`,
    `evidence-${block}.xlsx`,
  )
}

async function downloadPack() {
  packing.value = true
  try {
    await downloadFile(`/dengbao/evidence/pack?since=${encodeURIComponent(since.value)}`, packName())
  } finally { packing.value = false }
}

// ---------- R-C3 权限变更证据链 ----------
const permChain = ref<any[]>([])
const permLoading = ref(false)

async function loadPermChain() {
  permLoading.value = true
  try {
    permChain.value = (await call(api.http.get<never, any>('/dengbao/perm-chain', { params: { username: username.value, since: since.value } }))) || []
  } finally { permLoading.value = false }
}

async function downloadPermChain() {
  await downloadFile(`/dengbao/perm-chain/export?username=${encodeURIComponent(username.value)}&since=${encodeURIComponent(since.value)}`, 'permission-chain.xlsx')
}

// ---------- R-C1 等保资产台账 ----------
const assets = ref<any[]>([])
const assetLoading = ref(false)
const assetSaving = ref(false)
const assetTotal = computed(() => (assets.value || []).length)
const asset = reactive({
  name: '', category: 'application', level: '2', critical: 'normal',
  owner: '', department: '', addr: '',
})

async function loadAssets() {
  assetLoading.value = true
  try {
    assets.value = (await call(api.http.get<never, any>('/dengbao/assets'))) || []
  } finally { assetLoading.value = false }
}

async function saveAsset() {
  if (!asset.name.trim()) { ElMessage.warning('资产名称必填'); return }
  assetSaving.value = true
  try {
    await call(api.http.post<never, any>('/dengbao/assets', { ...asset }))
    ElMessage.success('资产已登记')
    asset.name = ''
    await loadAssets()
  } finally { assetSaving.value = false }
}

async function toggleAsset(row: any) {
  const next = row.status === 'active' ? 'retired' : 'active'
  try {
    await call(api.http.post<never, any>('/dengbao/assets', { ...row, status: next }))
    ElMessage.success('资产状态已更新')
    await loadAssets()
  } catch { /* 拦截器已提示 */ }
}

// ---------- R-C5 整改跟踪 ----------
const findings = ref<any[]>([])
const findingLoading = ref(false)
const findingSaving = ref(false)
const findingOpen = ref(0)
const findingLogs = ref<any[]>([])
const logVisible = ref(false)
const finding = reactive({
  item: '', description: '', level: 'medium', owner: '', dueDate: '', assetId: undefined as number | undefined,
})

async function loadFindings() {
  findingLoading.value = true
  try {
    const [list, stats] = await Promise.all([
      call(api.http.get<never, any>('/dengbao/findings')),
      call(api.http.get<never, any>('/dengbao/findings/stats')),
    ])
    findings.value = list || []
    findingOpen.value = stats?.open || 0
  } finally { findingLoading.value = false }
}

async function createFinding() {
  if (!finding.item.trim()) { ElMessage.warning('控制点/测评项必填'); return }
  findingSaving.value = true
  try {
    await call(api.http.post<never, any>('/dengbao/findings', { ...finding }))
    ElMessage.success('整改项已登记')
    finding.item = ''
    finding.description = ''
    await loadFindings()
  } finally { findingSaving.value = false }
}

async function notePrompt(text: string) {
  try { return (await ElMessageBox.prompt(text, '说明', { inputPlaceholder: '可选' })).value || '' } catch { return null }
}

async function verifyFinding(row: any) {
  const note = await notePrompt('复测结论（复测人将记录为当前登录用户，且不可与整改责任人相同）')
  if (note === null) return
  await act('/verify', row.id, note)
}

async function closeFinding(row: any) {
  const note = await notePrompt('关闭说明')
  if (note === null) return
  await act('/close', row.id, note)
}

async function reopenFinding(row: any) {
  const note = await notePrompt('退回原因')
  if (note === null) return
  await act('/reopen', row.id, note)
}

async function act(action: string, id: number, note: string) {
  try {
    await call(api.http.post<never, any>(`/dengbao/findings/${id}${action}`, { note }))
    ElMessage.success('已更新')
    await loadFindings()
  } catch { /* 拦截器已提示 */ }
}

async function showLog(row: any) {
  try {
    findingLogs.value = (await call(api.http.get<never, any>(`/dengbao/findings/${row.id}/log`))) || []
    logVisible.value = true
  } catch { /* 拦截器已提示 */ }
}

// ---------- R-C8 备份记录 ----------
const backups = ref<any[]>([])
const backupLoading = ref(false)
const backupSaving = ref(false)
const backupWindowDays = ref(30)
const backupLastSuccess = ref('')
const backupSummary = ref('')
const backup = reactive({
  source: 'database', tool: '', fileName: '', sizeBytes: undefined as number | undefined,
  checksum: '', status: 'success',
})

async function loadBackups() {
  backupLoading.value = true
  try {
    const [list, stats] = await Promise.all([
      call(api.http.get<never, any>('/dengbao/backups')),
      call(api.http.get<never, any>('/dengbao/backups/stats', { params: { days: backupWindowDays.value } })),
    ])
    backups.value = list || []
    backupWindowDays.value = stats?.windowDays || 30
    backupLastSuccess.value = stats?.lastSuccessAt || ''
    const by = stats?.byStatus || []
    backupSummary.value = by.map((x: any) => `${x.status} ${x.count} 次`).join('，') || '窗口内无记录'
  } finally { backupLoading.value = false }
}

async function addBackup() {
  backupSaving.value = true
  try {
    await call(api.http.post<never, any>('/dengbao/backups', { ...backup }))
    ElMessage.success('备份记录已登记')
    backup.fileName = ''
    backup.checksum = ''
    await loadBackups()
  } finally { backupSaving.value = false }
}

// packName 证据包文件名（与服务端命名同口径，便于留档比对）
function packName() {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `dengbao-evidence-${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}.zip`
}

// ---------- 公用 ----------
// 业务码非 0 视为失败：抛错由调用方提示，避免「假成功」。
// 注意：入参是 Promise（api.http.get(...) 直接传入，未 await），必须先 await 再解包。
// 兼容两种返回形态：拦截器已解包 body（r.code）与 axios 原始响应（r.data.code）。
async function call(res: any) {
  const r = await res
  const body = r?.code !== undefined ? r : (r?.data || r)
  if (body?.code !== 0) throw new Error(body?.message || '操作失败')
  return body?.data
}

// 导出走 axios（带 Bearer token），按 Content-Disposition 取文件名
async function downloadFile(path: string, fallback: string) {
  const res = await api.http.get<never, Blob>(path, { responseType: 'blob' })
  const filename = fallback
  const url = URL.createObjectURL(res)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

const categoryLabel = (v: string) => ({ application: '应用', system: '系统', device: '设备' } as any)[v] || v
const levelLabel = (v: string) => ({ high: '高', medium: '中', low: '低' } as any)[v] || v
const levelTag = (v: string) => ({ high: 'danger', medium: 'warning', low: 'info' } as any)[v] || 'info'
const statusLabel = (v: string) =>
  ({ open: '待整改', fixing: '整改中', verified: '复测通过', closed: '已关闭' } as any)[v] || v
const statusTag = (v: string) =>
  ({ open: 'danger', fixing: 'warning', verified: 'success', closed: 'info' } as any)[v] || 'info'
const sourceLabel = (v: string) => ({ database: '数据库', app: '应用', file: '文件' } as any)[v] || v


onMounted(() => {
  loadStatus().catch(() => {})
  loadBlocks().catch(() => {})
  loadPermChain().catch(() => {})
  loadAssets().catch(() => {})
  loadFindings().catch(() => {})
  loadBackups().catch(() => {})
})
</script>

<style scoped>
.toolbar-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
