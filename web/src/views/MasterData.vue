<template>
  <div>
    <el-alert type="info" :closable="false" show-icon
      title="主数据雏形（AK5.1–AK5.5）"
      description="预置客户 / 供应商 / 物料 / 组织 / 人员五类；每条记录有终身不变的稳定 ID（MD-xxx），供合同相对方、发票购销方引用。实体解析与去重（G6）属于治理底座深化范围，这里不做。" />
    <el-tabs v-model="tab" class="tabs">
      <el-tab-pane label="主数据记录" name="records">
        <div class="toolbar">
          <el-select v-model="query.mdType" placeholder="全部类型" clearable style="width: 160px">
            <el-option v-for="t in types" :key="t.code" :label="t.name" :value="t.code" />
          </el-select>
          <el-input v-model="query.keyword" placeholder="名称 / 编码 / 稳定ID" clearable style="width: 220px" @keyup.enter="loadList" />
          <el-button type="primary" @click="loadList">查询</el-button>
          <el-button type="success" @click="openDialog()">新增记录</el-button>
          <el-button @click="exportXlsx">导出 Excel</el-button>
        </div>
        <el-table :data="list" border stripe v-loading="loading">
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column prop="md_uid" label="稳定 ID" width="190" />
          <el-table-column label="类型" width="100">
            <template #default="{ row }">{{ typeLabel(row.md_type) }}</template>
          </el-table-column>
          <el-table-column prop="code" label="编码" width="130" />
          <el-table-column prop="name" label="名称" min-width="160" />
          <el-table-column prop="legal_entity_id" label="归属主体" width="100" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ row.status === 'active' ? '启用' : '停用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="扩展属性" min-width="140">
            <template #default="{ row }"><span class="attrs">{{ formatAttrs(row.attributes) }}</span></template>
          </el-table-column>
          <el-table-column label="操作" width="230" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openDialog(row)">编辑</el-button>
              <el-button link type="primary" @click="openLinkDialog(row)">建关联</el-button>
              <el-button link type="danger" @click="remove(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="实体关联" name="links">
        <div class="toolbar">
          <el-button type="primary" @click="openLinkDialog()">新增关联</el-button>
          <el-button @click="loadLinks">刷新</el-button>
          <span class="hint">例：供应商 → 所属组织（belongs_to），联系人 → 客户（contact_of）</span>
        </div>
        <el-table :data="links" border stripe>
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column label="起点" min-width="200">
            <template #default="{ row }">{{ row.from_name }} <span class="uid">{{ row.from_uid }}</span></template>
          </el-table-column>
          <el-table-column prop="relation" label="关系" width="140" />
          <el-table-column label="终点" min-width="200">
            <template #default="{ row }">{{ row.to_name }} <span class="uid">{{ row.to_uid }}</span></template>
          </el-table-column>
          <el-table-column label="操作" width="100">
            <template #default="{ row }">
              <el-button link type="danger" @click="removeLink(row)">解除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="dialog" :title="form.id ? '编辑主数据' : '新增主数据'" width="520px">
      <el-form label-width="100px">
        <el-form-item label="类型">
          <el-select v-model="form.mdType" :disabled="!!form.id" style="width: 100%">
            <el-option v-for="t in types" :key="t.code" :label="`${t.name}（${t.code}）`" :value="t.code" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="编码"><el-input v-model="form.code" placeholder="业务系统编码，可留空" /></el-form-item>
        <el-form-item label="归属主体">
          <el-select v-model="form.legalEntityId" style="width: 100%">
            <el-option label="跟随当前主体" :value="0" />
            <el-option v-for="e in entities" :key="e.id" :label="e.name" :value="e.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="form.status" style="width: 100%">
            <el-option label="启用" value="active" />
            <el-option label="停用" value="disabled" />
          </el-select>
        </el-form-item>
        <el-form-item label="扩展属性">
          <el-input v-model="attrText" type="textarea" :rows="3" placeholder='JSON，如 {"taxNo":"9131...","contact":"张三"}' />
        </el-form-item>
      </el-form>
      <div v-if="form.mdUid" class="uid-note">稳定 ID：{{ form.mdUid }}（不可修改，供下游引用）</div>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="linkDialog" title="建立主数据关联" width="480px">
      <el-form label-width="90px">
        <el-form-item label="起点">
          <el-select v-model="linkForm.fromId" filterable style="width: 100%">
            <el-option v-for="m in allRecords" :key="m.id" :label="`${m.name}（${m.md_uid}）`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="关系">
          <el-select v-model="linkForm.relation" style="width: 100%">
            <el-option label="belongs_to（属于）" value="belongs_to" />
            <el-option label="contact_of（联系人）" value="contact_of" />
            <el-option label="supplies（供货给）" value="supplies" />
          </el-select>
        </el-form-item>
        <el-form-item label="终点">
          <el-select v-model="linkForm.toId" filterable style="width: 100%">
            <el-option v-for="m in allRecords" :key="m.id" :label="`${m.name}（${m.md_uid}）`" :value="m.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="linkDialog = false">取消</el-button>
        <el-button type="primary" @click="saveLink">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import {
  api, type MasterDataTypeRow, type MasterDataRow, type MasterDataLinkRow, type LegalEntityRow,
} from '../api'

const tab = ref('records')
const loading = ref(false)
const types = ref<MasterDataTypeRow[]>([])
const entities = ref<LegalEntityRow[]>([])
const list = ref<MasterDataRow[]>([])
const allRecords = ref<MasterDataRow[]>([])
const links = ref<MasterDataLinkRow[]>([])
const query = reactive({ mdType: '', keyword: '' })

function typeLabel(code: string) {
  return types.value.find((t) => t.code === code)?.name || code
}
function formatAttrs(v: unknown) {
  if (!v) return '—'
  if (typeof v === 'string') return v.length > 60 ? v.slice(0, 60) + '…' : v
  return JSON.stringify(v)
}
async function loadList() {
  loading.value = true
  try {
    const res = await api.masterData({ mdType: query.mdType || undefined, keyword: query.keyword || undefined })
    list.value = res.data || []
  } finally {
    loading.value = false
  }
}
async function loadLinks() {
  const res = await api.masterDataLinks()
  links.value = res.data || []
}
async function loadRefs() {
  const [t, e, all] = await Promise.all([api.masterDataTypes(), api.legalEntities(), api.masterData()])
  types.value = t.data || []
  entities.value = e.data?.list || []
  allRecords.value = all.data || []
}
function exportXlsx() {
  window.location.href = api.masterDataExportUrl(query.mdType || undefined)
}

// ---------- 记录 CRUD ----------
const dialog = ref(false)
const attrText = ref('{}')
const form = reactive({ id: 0, mdUid: '', mdType: 'supplier', code: '', name: '', legalEntityId: 0, status: 'active' })
function openDialog(row?: MasterDataRow) {
  Object.assign(form, row
    ? { id: row.id, mdUid: row.md_uid, mdType: row.md_type, code: row.code, name: row.name,
        legalEntityId: row.legal_entity_id, status: row.status }
    : { id: 0, mdUid: '', mdType: types.value[0]?.code || 'supplier', code: '', name: '', legalEntityId: 0, status: 'active' })
  attrText.value = row ? (typeof row.attributes === 'string' ? row.attributes : JSON.stringify(row.attributes || {})) : '{}'
  dialog.value = true
}
async function save() {
  if (!form.name) return ElMessage.warning('名称必填')
  let attributes: Record<string, unknown> = {}
  try {
    attributes = attrText.value.trim() ? JSON.parse(attrText.value) : {}
  } catch {
    return ElMessage.warning('扩展属性不是合法 JSON')
  }
  const payload = { ...form, attributes }
  if (form.id) await api.masterDataUpdate(form.id, payload)
  else await api.masterDataCreate(payload)
  dialog.value = false
  ElMessage.success('已保存')
  await Promise.all([loadList(), loadRefs()])
}
async function remove(row: MasterDataRow) {
  await ElMessageBox.confirm(`删除「${row.name}」会同时解除其全部关联，确认？`, '提示', { type: 'warning' })
  await api.masterDataDelete(row.id)
  await Promise.all([loadList(), loadRefs(), loadLinks()])
}

// ---------- 关联 ----------
const linkDialog = ref(false)
const linkForm = reactive({ fromId: 0, toId: 0, relation: 'belongs_to' })
async function openLinkDialog(row?: MasterDataRow) {
  await loadRefs()
  Object.assign(linkForm, { fromId: row?.id || 0, toId: 0, relation: 'belongs_to' })
  linkDialog.value = true
}
async function saveLink() {
  if (!linkForm.fromId || !linkForm.toId) return ElMessage.warning('起点与终点都要选')
  await api.masterDataLinkCreate({ ...linkForm })
  linkDialog.value = false
  ElMessage.success('关联已建立')
  await loadLinks()
}
async function removeLink(row: MasterDataLinkRow) {
  await api.masterDataLinkDelete(row.id)
  await loadLinks()
}

onMounted(async () => {
  await Promise.all([loadRefs(), loadList(), loadLinks()])
})
</script>

<style scoped>
.tabs { padding: 0 4px; }
.toolbar { display: flex; align-items: center; gap: 10px; margin: 12px 0; flex-wrap: wrap; }
.hint { color: #909399; font-size: 12px; }
.attrs { color: #909399; font-size: 12px; }
.uid { color: #a8abb2; font-size: 12px; }
.uid-note { color: #67c23a; font-size: 12px; padding-left: 100px; margin-top: -8px; }
</style>
