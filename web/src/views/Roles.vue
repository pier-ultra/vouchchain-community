<template>
  <div>
    <el-alert type="warning" :closable="false" show-icon
      title="RBAC 角色与权限配置（产品② US-G1 / AG1.1–AG1.6）"
      description="自定义角色的功能权限、数据可见范围（全部/主体/部门/仅本人）与字段掩码在每次请求时实时校验；用户授权优先级高于登录页选择的角色。绑定当前登录用户会立刻影响本页面所有请求，请先配好写权限再绑定。" />

    <el-tabs v-model="tab" class="tabs">
      <el-tab-pane label="角色定义" name="roles">
        <div class="toolbar">
          <el-button type="primary" @click="openRole()">新增角色</el-button>
          <el-button @click="loadRoles">刷新</el-button>
          <span class="hint">四个预置角色沿用内置规则表，不可删除；自定义角色按权限清单判定。</span>
        </div>
        <el-table :data="roles" border stripe size="small">
          <el-table-column prop="code" label="编码" width="140" />
          <el-table-column prop="name" label="名称" width="130" />
          <el-table-column prop="description" label="说明" min-width="170" />
          <el-table-column label="预置" width="75">
            <template #default="{ row }"><el-tag size="small" :type="row.isBuiltin ? 'info' : 'success'">{{ row.isBuiltin ? '预置' : '自定义' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="数据范围" width="100">
            <template #default="{ row }">{{ scopeLabel(row.dataScope) }}</template>
          </el-table-column>
          <el-table-column label="权限项" min-width="180">
            <template #default="{ row }">
              <span v-if="row.isBuiltin" class="hint">内置规则表（全量按角色）</span>
              <span v-else>{{ row.permissions.length }} 项：{{ row.permissions.slice(0, 2).join('、') }}{{ row.permissions.length > 2 ? ' …' : '' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="字段掩码" width="120">
            <template #default="{ row }">{{ row.fieldMask.length ? row.fieldMask.join(', ') : '无' }}</template>
          </el-table-column>
          <el-table-column prop="userCount" label="绑定用户" width="90" />
          <el-table-column label="状态" width="80">
            <template #default="{ row }"><el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="操作" width="230" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openRole(row)">编辑</el-button>
              <el-button link type="warning" @click="toggleRole(row)">{{ row.enabled ? '停用' : '启用' }}</el-button>
              <el-button link type="danger" :disabled="row.isBuiltin" @click="removeRole(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="用户授权" name="bindings">
        <div class="toolbar">
          <el-input v-model="bindUser" placeholder="用户名（dev 模式下默认 dev）" style="width: 240px" />
          <el-select v-model="bindRole" placeholder="选择角色" style="width: 200px">
            <el-option v-for="r in roles" :key="r.code" :label="`${r.name} (${r.code})`" :value="r.code" />
          </el-select>
          <el-button type="primary" @click="bind('bind')">绑定</el-button>
          <el-button @click="bind('unbind')">解绑</el-button>
        </div>
        <el-table :data="bindings" border stripe size="small">
          <el-table-column prop="username" label="用户名" width="200" />
          <el-table-column prop="role_code" label="角色编码" width="200" />
          <el-table-column label="绑定时间" width="190">
            <template #default="{ row }">{{ fmt(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="110">
            <template #default="{ row }">
              <el-button link type="danger" @click="unbindRow(row)">解绑</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="当前有效权限" name="effective">
        <div class="toolbar"><el-button @click="loadEffective">重新读取</el-button><span class="hint">展示服务端对本次请求实际判定的角色与权限定义。</span></div>
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="当前用户名">{{ effective.username }}</el-descriptions-item>
          <el-descriptions-item label="请求头角色">{{ effective.headerRole || '-' }}</el-descriptions-item>
          <el-descriptions-item label="绑定角色">{{ effective.boundRole || '（无绑定）' }}</el-descriptions-item>
          <el-descriptions-item label="最终生效角色"><el-tag>{{ effective.effectiveRole }}</el-tag></el-descriptions-item>
        </el-descriptions>
        <pre class="json">{{ JSON.stringify(effective.definition, null, 2) }}</pre>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="roleDialog" :title="roleForm.id ? '编辑角色' : '新增自定义角色'" width="640px">
      <el-form :model="roleForm" label-width="110px">
        <el-form-item label="角色编码"><el-input v-model="roleForm.code" :disabled="!!roleForm.id" placeholder="如 finance_readonly" /></el-form-item>
        <el-form-item label="角色名称"><el-input v-model="roleForm.name" /></el-form-item>
        <el-form-item label="说明"><el-input v-model="roleForm.description" /></el-form-item>
        <el-form-item label="数据范围">
          <el-select v-model="roleForm.dataScope" style="width: 100%">
            <el-option label="全部数据 all" value="all" />
            <el-option label="仅当前法人主体 entity" value="entity" />
            <el-option label="同部门成员 dept" value="dept" />
            <el-option label="仅本人 self" value="self" />
          </el-select>
        </el-form-item>
        <el-form-item label="字段掩码">
          <el-select v-model="roleForm.fieldMask" multiple allow-create filterable style="width: 100%" placeholder="不可见字段名，如 amount / total">
            <el-option v-for="f in ['amount', 'total', 'tax', 'counter_party', 'buyer_name', 'seller_name']" :key="f" :label="f" :value="f" />
          </el-select>
        </el-form-item>
        <el-form-item label="功能权限">
          <el-select v-model="roleForm.permissions" multiple filterable style="width: 100%" placeholder="选择允许的操作（写入审计）">
            <el-option v-for="p in resources" :key="p.value" :label="`${p.label} · ${p.value}`" :value="p.value" />
          </el-select>
          <div class="hint">路径末尾带 * 表示通配该资源树下所有子路径，不带 * 为精确端点；前缀带 - 表示显式拒绝（优先级最高）；未列出的写操作默认拒绝，读操作默认放行。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="roleDialog = false">取消</el-button>
        <el-button type="primary" @click="saveRole">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { govApi, type RoleRow, type RoleBindingRow } from '../api'

const tab = ref('roles')
const roles = ref<RoleRow[]>([])
const bindings = ref<RoleBindingRow[]>([])
const resources = ref<{ value: string; method: string; path: string; label: string }[]>([])
const bindUser = ref('')
const bindRole = ref('')
const effective = reactive<{ username: string; headerRole: string; boundRole: string; effectiveRole: string; definition: unknown }>({
  username: '', headerRole: '', boundRole: '', effectiveRole: '', definition: {},
})
const roleDialog = ref(false)
const roleForm = reactive({
  id: 0, code: '', name: '', description: '', dataScope: 'all',
  permissions: [] as string[], fieldMask: [] as string[],
})

function fmt(v?: string) { return v ? String(v).replace('T', ' ').slice(0, 19) : '-' }
function scopeLabel(s: string) {
  return ({ all: '全部', entity: '本主体', dept: '本部门', self: '仅本人' } as Record<string, string>)[s] || s
}
async function loadAll() { await Promise.all([loadRoles(), loadBindings(), loadResources(), loadEffective()]) }
async function loadRoles() { roles.value = (await govApi.roles()).data || [] }
async function loadBindings() { bindings.value = (await govApi.roleBindings()).data || [] }
async function loadResources() { resources.value = (await govApi.roleResources()).data || [] }
async function loadEffective() {
  const res = await govApi.roleEffective()
  if (res.data) Object.assign(effective, res.data)
}
function openRole(row?: RoleRow) {
  Object.assign(roleForm, row ? {
    id: row.id, code: row.code, name: row.name, description: row.description,
    dataScope: row.dataScope, permissions: [...row.permissions], fieldMask: [...row.fieldMask],
  } : { id: 0, code: '', name: '', description: '', dataScope: 'all', permissions: [], fieldMask: [] })
  roleDialog.value = true
}
async function saveRole() {
  await govApi.roleSave({ ...roleForm })
  ElMessage.success('角色已保存，权限实时生效')
  roleDialog.value = false
  await Promise.all([loadRoles(), loadEffective()])
}
async function toggleRole(row: RoleRow) {
  await govApi.roleToggle(row.id)
  ElMessage.success(row.enabled ? '已停用' : '已启用')
  await loadRoles()
}
async function removeRole(row: RoleRow) {
  await govApi.roleDelete(row.id)
  ElMessage.success('已删除')
  await loadRoles()
}
async function bind(action: 'bind' | 'unbind') {
  if (!bindUser.value.trim() || !bindRole.value) { ElMessage.error('用户名与角色都要填'); return }
  if (action === 'bind' && bindUser.value.trim() === 'dev') {
    ElMessage.warning('绑定 dev 会立刻改变当前会话的有效角色，请确保已配置写权限')
  }
  await govApi.roleBindingSave(bindUser.value.trim(), bindRole.value, action)
  ElMessage.success(action === 'bind' ? '已绑定' : '已解绑')
  await Promise.all([loadBindings(), loadRoles(), loadEffective()])
}
async function unbindRow(row: RoleBindingRow) {
  await govApi.roleBindingSave(row.username, row.role_code, 'unbind')
  ElMessage.success('已解绑')
  await Promise.all([loadBindings(), loadRoles(), loadEffective()])
}
onMounted(loadAll)
</script>

<style scoped>
.tabs { margin-top: 12px }
.toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; flex-wrap: wrap }
.hint { font-size: 12px; color: var(--el-text-color-secondary) }
.json { background: var(--ch-surface-3); padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; margin-top: 12px }
</style>
