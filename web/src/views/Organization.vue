<template>
  <div>
    <el-card shadow="never" class="ctx-bar">
      <span class="ctx-label">当前法人主体</span>
      <el-select v-model="currentEntity" style="width: 260px" @change="switchEntity">
        <el-option label="全部主体（历史数据）" :value="0" />
        <el-option v-for="e in entities" :key="e.id" :label="indented(e)" :value="e.id" />
      </el-select>
      <el-checkbox v-model="includeSubtree" :disabled="currentEntity === 0" @change="switchSubtree">
        含下级主体
      </el-checkbox>
      <span class="ctx-hint">
        新建的合同 / 发票 / 应付 / 凭证自动打归属标记；勾选「含下级」后按父子树递归展开可见范围（AK3.2–AK3.4）。
      </span>
    </el-card>

    <el-tabs v-model="tab" class="tabs">
      <!-- AK3.1 法人主体 -->
      <el-tab-pane label="法人主体" name="entity">
        <div class="toolbar">
          <el-button type="primary" @click="openEntityDialog()">新增主体</el-button>
          <el-button @click="loadEntities">刷新</el-button>
        </div>
        <!-- 父子树：按 parent_id 递归拼好的树形表格，父子层级直接展开可见 -->
        <el-table :data="entityTree" row-key="id" :tree-props="{ children: 'children' }"
          default-expand-all border stripe v-loading="loading">
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column prop="code" label="编码" width="130" />
          <el-table-column prop="name" label="名称" min-width="180" />
          <el-table-column label="类型" width="100">
            <template #default="{ row }">{{ entityTypeLabel(row.entityType) }}</template>
          </el-table-column>
          <el-table-column label="层级路径" min-width="210">
            <template #default="{ row }">
              <span class="path-text" :title="row.path || row.name">{{ row.path || row.name }}</span>
              <el-tag v-if="row.descendantCount" size="small" type="info" class="desc-tag">↓{{ row.descendantCount }} 个下级</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ row.status === 'active' ? '启用' : '停用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="invoiceCount" label="本主体发票" width="105" />
          <el-table-column prop="totalInvoiceCount" label="含下级发票" width="105" />
          <el-table-column label="操作" width="220">
            <template #default="{ row }">
              <el-button link type="primary" @click="openEntityDialog(row)">编辑</el-button>
              <el-button link type="warning" @click="toggleEntity(row)">
                {{ row.status === 'active' ? '停用' : '启用' }}
              </el-button>
              <el-button link type="danger" @click="removeEntity(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- AK2.1 / AK2.2 / AK2.4 组织树 -->
      <el-tab-pane label="组织树" name="tree">
        <div class="toolbar">
          <el-button type="primary" @click="openNodeDialog(0)">新增根节点</el-button>
          <el-button @click="loadTree">刷新</el-button>
          <span class="ctx-hint">括号为该节点及子树的挂载人数；点击节点后在右侧维护。</span>
        </div>
        <el-row :gutter="16">
          <el-col :span="10">
            <el-tree
              :data="tree" :props="{ children: 'children', label: 'name' }"
              node-key="id" default-expand-all highlight-current @node-click="onNodeClick"
            >
              <template #default="{ data }">
                <span>
                  <el-tag size="small" :type="data.nodeType === 'position' ? 'warning' : 'info'" style="margin-right: 6px">
                    {{ data.nodeType === 'position' ? '岗位' : '部门' }}
                  </el-tag>
                  {{ data.name }}
                  <span class="tree-count">({{ data.totalMemberCount }} 人)</span>
                  <span class="tree-code">{{ data.code }}</span>
                </span>
              </template>
            </el-tree>
            <el-empty v-if="!tree.length" description="暂无组织节点，先新增根节点或导入 CSV" />
          </el-col>
          <el-col :span="14">
            <el-card shadow="never" v-if="selected.id">
              <template #header>
                <div class="card-head">
                  <span>{{ selected.name }}（#{{ selected.id }}）</span>
                  <div>
                    <el-button size="small" type="primary" @click="openNodeDialog(selected.id)">新增子节点</el-button>
                    <el-button size="small" @click="editSelected">编辑</el-button>
                    <el-button size="small" type="danger" @click="removeNode(selected)">删除</el-button>
                  </div>
                </div>
              </template>
              <el-descriptions :column="2" border size="small">
                <el-descriptions-item label="编码">{{ selected.code || '—' }}</el-descriptions-item>
                <el-descriptions-item label="类型">{{ selected.nodeType === 'position' ? '岗位' : '部门' }}</el-descriptions-item>
                <el-descriptions-item label="层级">第 {{ (selected.depth || 0) + 1 }} 层</el-descriptions-item>
                <el-descriptions-item label="子节点">{{ selected.subtreeNodes || 0 }} 个（递归）</el-descriptions-item>
                <el-descriptions-item label="直接挂载">{{ selected.memberCount }} 人</el-descriptions-item>
                <el-descriptions-item label="含子树">{{ selected.totalMemberCount }} 人</el-descriptions-item>
                <el-descriptions-item label="父子路径" :span="2">{{ selected.path || selected.name }}</el-descriptions-item>
              </el-descriptions>
              <div class="member-head">
                节点人员（AK2.3）
                <el-radio-group v-model="memberScope" size="small" style="margin-left: 12px" @change="loadMembers">
                  <el-radio-button value="direct">本节点</el-radio-button>
                  <el-radio-button value="subtree">含子树（递归）</el-radio-button>
                </el-radio-group>
              </div>
              <el-alert v-if="memberScope === 'subtree' && subtree" type="info" :closable="false" class="subtree-hint"
                :title="`递归展开 ${subtree.nodeCount} 个节点、${subtree.members.length} 名成员`" />
              <div class="member-add">
                <el-input v-model="newUsername" placeholder="用户名，如 zhangsan" style="width: 220px" />
                <el-button type="primary" @click="attachMember">挂载</el-button>
              </div>
              <el-table :data="shownMembers" size="small" border>
                <el-table-column prop="id" label="ID" width="70" />
                <el-table-column prop="username" label="用户名" />
                <el-table-column label="来源节点" min-width="140">
                  <template #default="{ row }">
                    {{ row.nodeName }}
                    <el-tag v-if="!row.isDirect" size="small" type="info">下级</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="90">
                  <template #default="{ row }">
                    <el-button link type="danger" @click="removeMember(row)">卸载</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
            <el-empty v-else description="点击左侧节点查看详情与人员挂载" />
          </el-col>
        </el-row>
      </el-tab-pane>

      <!-- AK2.5 CSV 导入 -->
      <el-tab-pane label="CSV 批量导入" name="import">
        <el-alert type="info" :closable="false" show-icon
          title="列顺序：code,name,node_type,parent_code,sort_order（或 编码,名称,类型,上级编码,排序）"
          description="首行表头可省略；node_type 支持 dept/部门/position/岗位；parent_code 引用同文件中已先出现的节点编码或库中已有编码。CSV 为 HR 系统批量导入的占位接口，首版也可手工建。" />
        <div class="toolbar">
          <input type="file" accept=".csv,text/csv" @change="onCsvPick" />
          <el-button type="primary" :disabled="!csvFile" @click="importCsv">导入</el-button>
        </div>
        <div v-if="importResult" class="import-result">
          <p>成功导入 <b>{{ importResult.created }}</b> 个节点，问题 {{ importResult.errors.length }} 条：</p>
          <ul><li v-for="(e, i) in importResult.errors" :key="i">{{ e }}</li></ul>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 主体新增 / 编辑 -->
    <el-dialog v-model="entityDialog" :title="entityForm.id ? '编辑主体' : '新增主体'" width="480px">
      <el-form label-width="90px">
        <el-form-item label="编码"><el-input v-model="entityForm.code" :disabled="!!entityForm.id" /></el-form-item>
        <el-form-item label="名称"><el-input v-model="entityForm.name" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="entityForm.entityType" style="width: 100%">
            <el-option label="总公司" value="headquarters" />
            <el-option label="子公司" value="subsidiary" />
            <el-option label="分支机构" value="branch" />
          </el-select>
        </el-form-item>
        <el-form-item label="上级主体">
          <el-select v-model="entityForm.parentId" style="width: 100%" clearable>
            <el-option label="无" :value="0" />
            <el-option v-for="e in entities.filter((x) => x.id !== entityForm.id)" :key="e.id" :label="indented(e)" :value="e.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="entityDialog = false">取消</el-button>
        <el-button type="primary" @click="saveEntity">保存</el-button>
      </template>
    </el-dialog>

    <!-- 组织节点新增 / 编辑 -->
    <el-dialog v-model="nodeDialog" :title="nodeForm.id ? '编辑节点' : '新增节点'" width="480px">
      <el-form label-width="90px">
        <el-form-item label="名称"><el-input v-model="nodeForm.name" /></el-form-item>
        <el-form-item label="编码"><el-input v-model="nodeForm.code" placeholder="CSV 上级引用用" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="nodeForm.nodeType" style="width: 100%">
            <el-option label="部门" value="dept" />
            <el-option label="岗位" value="position" />
          </el-select>
        </el-form-item>
        <el-form-item label="排序"><el-input-number v-model="nodeForm.sortOrder" :min="0" /></el-form-item>
        <el-form-item label="归属主体">
          <el-select v-model="nodeForm.legalEntityId" style="width: 100%">
            <el-option label="未指定（跟随当前主体）" :value="0" />
            <el-option v-for="e in entities" :key="e.id" :label="e.name" :value="e.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nodeDialog = false">取消</el-button>
        <el-button type="primary" @click="saveNode">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api, type LegalEntityRow, type OrgNodeRow, type OrgMemberRow, type OrgSubtreeRow } from '../api'

const tab = ref('entity')
const loading = ref(false)
const entities = ref<LegalEntityRow[]>([])      // 扁平清单：下拉框与编辑表单
const entityTree = ref<LegalEntityRow[]>([])    // 递归树：树形表格展开
const currentEntity = ref<number>(Number(localStorage.getItem('ch_entity') || 0))
const includeSubtree = ref(false)
const tree = ref<OrgNodeRow[]>([])
const selected = ref<Partial<OrgNodeRow>>({})
const nodeMembers = ref<OrgMemberRow[]>([])
const memberScope = ref<'direct' | 'subtree'>('direct')
const subtree = ref<OrgSubtreeRow | null>(null)
const newUsername = ref('')

// 两种视图统一成一份表格数据，差别只在「递归范围」与来源节点标注
const shownMembers = computed(() => {
  if (memberScope.value === 'direct') {
    return nodeMembers.value.map((m) => ({
      id: m.id, username: m.username, nodeName: selected.value.name || '', isDirect: true,
    }))
  }
  return subtree.value?.members || []
})

async function loadEntities() {
  const res = await api.legalEntities()
  entities.value = res.data?.list || []
  entityTree.value = res.data?.tree || []
}
async function loadContext() {
  // 主体表格以 legalEntities 接口（含树形与归属数量）为准，这里只同步当前上下文，避免并发覆盖
  const res = await api.context()
  if (res.data) {
    includeSubtree.value = !!res.data.includeSubtree
    if (!localStorage.getItem('ch_entity')) currentEntity.value = res.data.legalEntityId
  }
}
async function switchEntity(id: number) {
  await api.contextSwitch(id, includeSubtree.value)
  localStorage.setItem('ch_entity', String(id))
  ElMessage.success(id === 0 ? '已切回全部主体' : '当前主体已切换，列表与新建数据按该主体过滤')
  await loadAll()
}
async function switchSubtree(v: boolean) {
  await api.contextSwitch(currentEntity.value, v)
  ElMessage.success(v ? '已按父子树递归带上所有下级主体' : '已恢复为仅看当前主体')
  await loadAll()
}
function indented(e: LegalEntityRow) {
  const d = e.depth || 0
  return '　'.repeat(d) + (d > 0 ? '└ ' : '') + e.name
}
function entityTypeLabel(t: string) {
  return { headquarters: '总公司', subsidiary: '子公司', branch: '分支机构' }[t] || t
}

// ---------- 法人主体 ----------
const entityDialog = ref(false)
const entityForm = reactive({ id: 0, code: '', name: '', entityType: 'subsidiary', parentId: 0 })
function openEntityDialog(row?: LegalEntityRow) {
  Object.assign(entityForm, row
    ? { id: row.id, code: row.code, name: row.name, entityType: row.entityType, parentId: row.parentId }
    : { id: 0, code: '', name: '', entityType: 'subsidiary', parentId: 0 })
  entityDialog.value = true
}
async function saveEntity() {
  if (!entityForm.code || !entityForm.name) return ElMessage.warning('编码与名称必填')
  if (entityForm.id) await api.legalEntityUpdate(entityForm.id, { ...entityForm })
  else await api.legalEntityCreate({ ...entityForm })
  entityDialog.value = false
  ElMessage.success('已保存')
  await loadEntities()
}
async function toggleEntity(row: LegalEntityRow) {
  await api.legalEntityUpdate(row.id, { status: row.status === 'active' ? 'disabled' : 'active' })
  await loadEntities()
}
async function removeEntity(row: LegalEntityRow) {
  await ElMessageBox.confirm(`确认删除主体「${row.name}」？已有业务数据的主体不允许删除。`, '提示', { type: 'warning' })
  await api.legalEntityDelete(row.id)
  await loadEntities()
}

// ---------- 组织树 ----------
const nodeDialog = ref(false)
const nodeForm = reactive({ id: 0, parentId: 0, name: '', code: '', nodeType: 'dept', sortOrder: 0, legalEntityId: 0 })
function openNodeDialog(parentId: number, row?: OrgNodeRow) {
  const src = row as OrgNodeRow | undefined
  Object.assign(nodeForm, src
    ? { id: src.id, parentId: src.parentId, name: src.name, code: src.code,
        nodeType: src.nodeType, sortOrder: src.sortOrder, legalEntityId: src.legalEntityId }
    : { id: 0, parentId, name: '', code: '', nodeType: 'dept', sortOrder: 0, legalEntityId: currentEntity.value })
  nodeDialog.value = true
}
function editSelected() {
  openNodeDialog(selected.value.parentId || 0, selected.value as OrgNodeRow)
}
async function saveNode() {
  if (!nodeForm.name) return ElMessage.warning('节点名称必填')
  if (nodeForm.id) await api.orgNodeUpdate(nodeForm.id, { ...nodeForm })
  else await api.orgNodeCreate({ ...nodeForm })
  nodeDialog.value = false
  ElMessage.success('已保存')
  await loadTree()
}
async function removeNode(row: Partial<OrgNodeRow>) {
  await ElMessageBox.confirm(`确认删除节点「${row.name}」？`, '提示', { type: 'warning' })
  await api.orgNodeDelete(row.id as number)
  selected.value = {}
  await loadTree()
}
async function loadTree() {
  const res = await api.orgTree(currentEntity.value || undefined)
  tree.value = res.data?.tree || []
}
async function loadMembers() {
  if (!selected.value.id) return
  if (memberScope.value === 'subtree') {
    const res = await api.orgSubtree(selected.value.id)
    subtree.value = res.data || null
  } else {
    const res = await api.orgMembers(selected.value.id)
    nodeMembers.value = res.data || []
  }
}
async function onNodeClick(data: OrgNodeRow) {
  selected.value = data
  await loadMembers()
}
async function attachMember() {
  if (!selected.value.id || !newUsername.value.trim()) return ElMessage.warning('请先选择节点并填写用户名')
  await api.orgMemberAttach(selected.value.id, newUsername.value.trim())
  newUsername.value = ''
  ElMessage.success('已挂载')
  await onNodeClick(selected.value as OrgNodeRow)
  await loadTree()
}
async function removeMember(row: { id: number }) {
  await api.orgMemberRemove(row.id)
  await loadMembers()
  await loadTree()
}

// ---------- CSV 导入 ----------
const csvFile = ref<File | null>(null)
const importResult = ref<{ created: number; errors: string[] } | null>(null)
function onCsvPick(e: Event) {
  const input = e.target as HTMLInputElement
  csvFile.value = input.files?.[0] || null
}
async function importCsv() {
  if (!csvFile.value) return
  const res = await api.orgNodeImport(csvFile.value)
  importResult.value = res.data || { created: 0, errors: ['无返回'] }
  ElMessage.success(`导入完成，成功 ${importResult.value.created} 个节点`)
  await loadTree()
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadEntities(), loadContext(), loadTree()])
  } finally {
    loading.value = false
  }
}
onMounted(loadAll)
</script>

<style scoped>
.ctx-bar { margin-bottom: 12px; }
.ctx-label { font-weight: 600; margin-right: 12px; }
.ctx-hint { color: #909399; font-size: 12px; margin-left: 12px; }
.tabs { padding: 0 4px; }
.toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; flex-wrap: wrap; }
.tree-count { color: #409eff; font-size: 12px; margin-left: 4px; }
.tree-code { color: #a8abb2; font-size: 12px; margin-left: 8px; }
.path-text {
  display: inline-block;
  max-width: 132px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
  color: #606266;
  font-size: 12px;
}
.desc-tag { margin-left: 6px; }
.subtree-hint { margin-bottom: 10px; }
.card-head { display: flex; justify-content: space-between; align-items: center; }
.member-head { margin: 14px 0 8px; font-weight: 600; }
.member-add { display: flex; gap: 8px; margin-bottom: 10px; }
.import-result { margin-top: 12px; color: #606266; }
.import-result li { color: #e6a23c; font-size: 12px; }
</style>
