<template>
  <div>
    <el-alert type="info" :closable="false" show-icon
      title="SCIM 2.0 身份同步（产品② 治理底座）"
      description="身份系统（Casdoor / 飞书 / AD）加人、调组、离职时主动推送到 /scim/v2，本系统自动开户、重写角色绑定、即时禁用账号。推送用独立 Bearer token，与用户登录 JWT 是两套凭据。" />

    <el-alert v-if="status && !status.configured" class="warn" type="error" :closable="false" show-icon
      title="尚未启用：SCIM 端点当前全部返回 503"
      description="部署时设置环境变量 SCIM_TOKEN=<自定义长随机串> 再重启服务即可，源码与数据库都不用改。未配置时绝不无鉴权开放开户接口。" />

    <el-row :gutter="12" class="cards">
      <el-col :span="6"><el-card shadow="never"><div class="k">接入状态</div><div class="v" :class="status?.configured ? 'ok' : 'bad'">{{ status?.configured ? '已就绪' : '未配置' }}</div><div class="sub">{{ status?.authMode === 'sso' ? 'SSO 生产态' : 'dev 演示态' }} · {{ status?.baseUrl }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="k">同步账号</div><div class="v">{{ status?.users ?? 0 }}</div><div class="sub">启用 {{ status?.activeUsers ?? 0 }} · 禁用 {{ (status?.users ?? 0) - (status?.activeUsers ?? 0) }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="k">组 / 映射</div><div class="v">{{ status?.groups ?? 0 }} <span class="slash">/</span> {{ status?.roleMappings ?? 0 }}</div><div class="sub">组名 → 本系统角色</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="k">同步记录</div><div class="v">{{ status?.logs ?? 0 }}</div><div class="sub">{{ status?.lastSync?.username ? `最近：${status.lastSync.username} · ${status.lastSync.operation}` : '尚无推送' }}</div></el-card></el-col>
    </el-row>

    <el-tabs v-model="tab" class="tabs">
      <el-tab-pane label="同步账号" name="users">
        <div class="toolbar">
          <el-button @click="loadAll">刷新</el-button>
          <span class="hint">「生效角色」由组名经映射表算出，写入角色绑定；SCIM 只覆盖自己写的绑定，不冲掉手工授权。</span>
        </div>
        <el-table :data="users" border stripe size="small" v-loading="loading">
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column prop="username" label="用户名" width="150" />
          <el-table-column prop="displayName" label="姓名" width="130" />
          <el-table-column prop="email" label="邮箱" min-width="170" />
          <el-table-column label="所属组" min-width="160">
            <template #default="{ row }">
              <el-tag v-for="g in row.groups" :key="g" size="small" class="tag">{{ g }}</el-tag>
              <span v-if="!row.groups.length" class="hint">—</span>
            </template>
          </el-table-column>
          <el-table-column label="生效角色" min-width="140">
            <template #default="{ row }">
              <el-tag v-for="c in row.roles" :key="c" size="small" type="success" class="tag">{{ c }}</el-tag>
              <span v-if="!row.roles.length" class="hint">无（组未映射）</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }"><el-tag size="small" :type="row.active ? 'success' : 'danger'">{{ row.active ? '启用' : '禁用' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="最后同步" width="170">
            <template #default="{ row }">{{ fmt(row.lastSyncedAt) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <el-button link type="danger" :disabled="currentRole() !== 'admin'" @click="purgeUser(row)">彻底删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="组与角色映射" name="maps">
        <div class="toolbar">
          <el-button type="primary" :disabled="currentRole() !== 'admin'" @click="openMap()">新增映射</el-button>
          <el-button @click="loadAll">刷新</el-button>
          <span class="hint">组名先查映射表；没配映射时，组名本身等于角色编码（如 legal）也生效。改完立刻重算存量用户角色。</span>
        </div>
        <el-row :gutter="12">
          <el-col :span="10">
            <div class="sub-title">身份系统推来的组</div>
            <el-table :data="groups" border stripe size="small">
              <el-table-column prop="displayName" label="组名" min-width="130" />
              <el-table-column label="成员" min-width="130">
                <template #default="{ row }">
                  <el-tag v-for="m in row.members" :key="m" size="small" class="tag">{{ m }}</el-tag>
                  <span v-if="!row.members.length" class="hint">空</span>
                </template>
              </el-table-column>
            </el-table>
          </el-col>
          <el-col :span="14">
            <div class="sub-title">组名 → 角色映射</div>
            <el-table :data="maps" border stripe size="small">
              <el-table-column prop="groupName" label="组名" width="160" />
              <el-table-column label="角色" width="140"><template #default="{ row }"><el-tag size="small" type="success">{{ row.roleCode }}</el-tag></template></el-table-column>
              <el-table-column prop="note" label="说明" min-width="160" />
              <el-table-column label="操作" width="130" fixed="right">
                <template #default="{ row }">
                  <el-button link type="primary" :disabled="currentRole() !== 'admin'" @click="openMap(row)">编辑</el-button>
                  <el-button link type="danger" :disabled="currentRole() !== 'admin'" @click="removeMap(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-col>
        </el-row>
      </el-tab-pane>

      <el-tab-pane label="同步日志" name="logs">
        <div class="toolbar"><el-button @click="loadAll">刷新</el-button><span class="hint">每次推送留痕，含操作、资源、结果状态与原始报文（不含 Bearer token）。</span></div>
        <el-table :data="logs" border stripe size="small">
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column label="操作" width="100"><template #default="{ row }"><el-tag size="small">{{ row.operation }}</el-tag></template></el-table-column>
          <el-table-column prop="resourceType" label="资源" width="90" />
          <el-table-column prop="username" label="用户" width="140" />
          <el-table-column label="结果" width="90"><template #default="{ row }"><el-tag size="small" :type="row.httpStatus < 400 ? 'success' : 'danger'">{{ row.httpStatus }}</el-tag></template></el-table-column>
          <el-table-column prop="message" label="说明" min-width="220" show-overflow-tooltip />
          <el-table-column label="时间" width="165"><template #default="{ row }">{{ fmt(row.createdAt) }}</template></el-table-column>
          <el-table-column label="报文" width="80"><template #default="{ row }"><el-button link type="primary" @click="showPayload(row)">查看</el-button></template></el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="接入说明" name="guide">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="Base URL">{{ origin }}{{ status?.baseUrl || '/scim/v2' }}</el-descriptions-item>
          <el-descriptions-item label="鉴权">Authorization: Bearer &lt;SCIM_TOKEN&gt;</el-descriptions-item>
          <el-descriptions-item label="启用方式">环境变量 SCIM_TOKEN=&lt;自定义长随机串&gt;（不落库、不写配置文件）</el-descriptions-item>
          <el-descriptions-item label="内容类型">application/scim+json（application/json 也可）</el-descriptions-item>
          <el-descriptions-item label="离职处理">PATCH active=false 或 DELETE /Users/{id}（软删为禁用），已签发 JWT 立即失效</el-descriptions-item>
          <el-descriptions-item label="角色来源">User.groups / Group.members → 组名映射表 → rbac_user_role（source=scim）</el-descriptions-item>
        </el-descriptions>
        <div class="sub-title">开户示例（把 &lt;...&gt; 换成实际值）</div>
        <pre class="code">curl -X POST {{ origin }}{{ status?.baseUrl || '/scim/v2' }}/Users \
  -H "Authorization: Bearer &lt;SCIM_TOKEN&gt;" \
  -H "Content-Type: application/scim+json" \
  -d '{
    "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
    "externalId": "casdoor-1001", "userName": "zhangsan",
    "displayName": "张三", "emails": [{"value": "z@example.com", "primary": true}],
    "active": true, "groups": [{"display": "法务组"}]
  }'</pre>
        <div class="sub-title">离职示例</div>
        <pre class="code">curl -X PATCH {{ origin }}{{ status?.baseUrl || '/scim/v2' }}/Users/&lt;id&gt; \
  -H "Authorization: Bearer &lt;SCIM_TOKEN&gt;" \
  -H "Content-Type: application/scim+json" \
  -d '{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
       "Operations":[{"op":"replace","path":"active","value":false}]}'</pre>
        <div class="sub-title">已实现端点</div>
        <div class="endpoints"><el-tag v-for="e in status?.endpoints || []" :key="e" size="small" class="tag">{{ e }}</el-tag></div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="mapDlg" title="组名 → 角色映射" width="480px">
      <el-form label-width="90px">
        <el-form-item label="组名"><el-input v-model="mapForm.groupName" placeholder="身份系统里的组 displayName，如 法务组" /></el-form-item>
        <el-form-item label="角色">
          <el-select v-model="mapForm.roleCode" style="width: 100%" placeholder="选择本系统角色">
            <el-option v-for="r in roleOptions" :key="r.code" :label="`${r.name} (${r.code})`" :value="r.code" />
          </el-select>
        </el-form-item>
        <el-form-item label="说明"><el-input v-model="mapForm.note" placeholder="选填" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="mapDlg = false">取消</el-button>
        <el-button type="primary" @click="saveMap">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="payloadDlg" title="原始推送报文" width="680px">
      <pre class="code">{{ prettyPayload }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { govApi, type ScimStatusRow, type ScimUserRow, type ScimGroupRow, type ScimLogRow, type ScimRoleMapRow, type RoleRow } from '../api'
import { currentRole } from '../rbac'

const tab = ref('users')
const loading = ref(false)
const status = ref<ScimStatusRow | null>(null)
const users = ref<ScimUserRow[]>([])
const groups = ref<ScimGroupRow[]>([])
const logs = ref<ScimLogRow[]>([])
const maps = ref<ScimRoleMapRow[]>([])
const roleOptions = ref<RoleRow[]>([])
const origin = location.origin

const mapDlg = ref(false)
const mapForm = reactive<{ id: number; groupName: string; roleCode: string; note: string }>({ id: 0, groupName: '', roleCode: '', note: '' })

const payloadDlg = ref(false)
const prettyPayload = ref('')

async function loadAll() {
  loading.value = true
  try {
    const [s, u, g, l, m, ro] = await Promise.all([
      govApi.scimStatus(), govApi.scimUsers(), govApi.scimGroups(),
      govApi.scimLogs(50), govApi.scimRoleMaps(), govApi.roles(),
    ])
    status.value = s.data || null
    users.value = u.data || []
    groups.value = g.data || []
    logs.value = l.data || []
    maps.value = m.data || []
    roleOptions.value = (ro.data || []).filter((x) => x.enabled)
  } finally {
    loading.value = false
  }
}

function fmt(s: string) {
  return s ? String(s).replace('T', ' ').slice(0, 19) : '—'
}

function openMap(row?: ScimRoleMapRow) {
  mapForm.id = row?.id || 0
  mapForm.groupName = row?.groupName || ''
  mapForm.roleCode = row?.roleCode || ''
  mapForm.note = row?.note || ''
  mapDlg.value = true
}

async function saveMap() {
  if (!mapForm.groupName.trim() || !mapForm.roleCode) {
    ElMessage.error('组名与角色必填')
    return
  }
  await govApi.scimRoleMapSave({ ...mapForm })
  ElMessage.success('映射已保存，存量用户角色已重算')
  mapDlg.value = false
  await loadAll()
}

async function removeMap(row: ScimRoleMapRow) {
  await ElMessageBox.confirm(`删除映射「${row.groupName} → ${row.roleCode}」？相关用户角色会即时重算。`, '确认', { type: 'warning' })
  await govApi.scimRoleMapDelete(row.id)
  ElMessage.success('已删除')
  await loadAll()
}

async function purgeUser(row: ScimUserRow) {
  await ElMessageBox.confirm(
    `彻底删除同步账号「${row.username}」？SCIM 的 DELETE 是软删（保留轨迹），这里是物理清库，不可恢复。`,
    '确认', { type: 'warning' },
  )
  await govApi.scimUserPurge(row.id)
  ElMessage.success('已彻底删除')
  await loadAll()
}

function showPayload(row: ScimLogRow) {
  try {
    prettyPayload.value = JSON.stringify(JSON.parse(row.payload || '{}'), null, 2)
  } catch {
    prettyPayload.value = row.payload || '（无报文）'
  }
  payloadDlg.value = true
}

onMounted(loadAll)
</script>

<style scoped>
.warn, .tabs, .cards { margin-top: 12px; }
.cards .k { color: #909399; font-size: 12px; }
.cards .v { font-size: 26px; font-weight: 600; line-height: 1.5; color: #303133; }
.cards .v.ok { color: #67c23a; }
.cards .v.bad { color: #f56c6c; }
.cards .sub { color: #a8abb2; font-size: 12px; min-height: 18px; }
.slash { color: #c0c4cc; font-weight: 400; }
.toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.hint { color: #909399; font-size: 12px; }
.tag { margin: 2px 4px 2px 0; }
.sub-title { margin: 14px 0 8px; font-weight: 600; font-size: 14px; color: var(--el-text-color-primary); }
.code { background: var(--ch-surface-3); border: 1px solid var(--ch-line-soft); border-radius: 4px; padding: 12px; font-size: 12px; line-height: 1.7; white-space: pre-wrap; word-break: break-all; color: var(--el-text-color-primary); }
.endpoints { line-height: 2.2; }
</style>
