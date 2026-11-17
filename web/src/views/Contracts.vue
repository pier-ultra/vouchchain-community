<template>
  <el-card>
    <div class="toolbar">
      <span class="card-title">合同台账</span>
      <el-button v-if="can('contract:edit')" type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon>
        新增合同
      </el-button>
    </div>

    <!-- 筛选区：v13 AC6.1（按状态筛选 + 关键词搜索） -->
    <div class="filters">
      <el-select v-model="query.status" placeholder="全部状态" clearable style="width: 140px" @change="load">
        <el-option label="草稿" value="draft" />
        <el-option label="生效中" value="active" />
        <el-option label="已过期" value="expired" />
        <el-option label="已终止" value="terminated" />
      </el-select>
      <el-input
        v-model="query.keyword"
        placeholder="搜合同名称 / 对方主体"
        clearable
        style="width: 240px"
        @keyup.enter="load"
        @clear="load"
      />
      <el-button type="primary" plain @click="load">
        <el-icon><Search /></el-icon>
        查询
      </el-button>
      <el-button @click="resetFilters">重置</el-button>
    </div>

    <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="title" label="合同名称" min-width="180" />
      <el-table-column prop="counterParty" label="对方主体" min-width="150" />
      <el-table-column label="金额" width="140">
        <template #default="{ row }">
          <span>¥ {{ fmtAmount(row.amount) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="currency" label="币种" width="80" />
      <el-table-column prop="signDate" label="签订日期" width="110" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusMap[row.status]?.type || 'info'">
            {{ statusMap[row.status]?.label || row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column v-if="can('contract:edit') || can('contract:delete')" label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button v-if="can('contract:edit')" link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button v-if="can('contract:delete')" link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        v-model:current-page="query.page"
        v-model:page-size="query.pageSize"
        :total="total"
        :page-sizes="[10, 20, 50]"
        layout="total, sizes, prev, pager, next"
        @current-change="load"
        @size-change="load"
      />
    </div>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑合同' : '新增合同'" width="520px">
      <el-form label-width="90px">
        <el-form-item label="合同名称" required>
          <el-input v-model="form.title" placeholder="如：测试合同A" />
        </el-form-item>
        <el-form-item label="对方主体">
          <el-input v-model="form.counterParty" placeholder="如：乙方公司" @blur="checkParty" />
          <el-alert v-if="partyTip" :type="partyTip.level === 'high' ? 'error' : partyTip.level === 'medium' ? 'warning' : 'info'"
            :closable="false" show-icon class="party-tip">
            <b>签约前提示 · {{ partyTip.label }}</b>（风险分 {{ partyTip.score }}）：
            {{ partyTip.contractCount }} 份历史合同、按时履约 {{ partyTip.fulfillRate }}%、
            争议条款 {{ partyTip.disputeClauseCount }} 条。{{ partyTip.warning }}
            <router-link to="/party-profiles" class="party-link">查看完整画像 →</router-link>
          </el-alert>
        </el-form-item>
        <el-form-item label="金额">
          <el-input-number v-model="form.amount" :min="0" :precision="2" style="width: 200px" />
        </el-form-item>
        <el-form-item label="币种">
          <el-select v-model="form.currency" style="width: 200px">
            <el-option label="CNY 人民币" value="CNY" />
            <el-option label="USD 美元" value="USD" />
            <el-option label="EUR 欧元" value="EUR" />
          </el-select>
        </el-form-item>
        <el-form-item label="签订日期">
          <el-date-picker v-model="form.signDate" type="date" value-format="YYYY-MM-DD" style="width: 200px" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="form.status" style="width: 200px">
            <el-option label="草稿" value="draft" />
            <el-option label="生效中" value="active" />
            <el-option label="已过期" value="expired" />
            <el-option label="已终止" value="terminated" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, govApi, type Contract } from '../api'
import { can } from '../rbac'

// AC8.5 签约前提示：录入相对方后即时拉取内部画像，不阻断保存
const partyTip = ref<{
  level: string; label: string; score: number; contractCount: number
  fulfillRate: string; disputeClauseCount: number; warning: string
} | null>(null)
async function checkParty() {
  const name = form.counterParty.trim()
  if (!name) {
    partyTip.value = null
    return
  }
  const res = await govApi.partyDetail(name)
  const p = res.data?.profile
  if (!p || !p.contractCount) {
    partyTip.value = null
    return
  }
  partyTip.value = {
    level: p.riskLevel, label: res.data?.riskLabel || '', score: p.riskScore,
    contractCount: p.contractCount, fulfillRate: p.fulfillRate.toFixed(0),
    disputeClauseCount: p.disputeClauseCount, warning: p.sampleWarning || '',
  }
}

const list = ref<Contract[]>([])
const total = ref(0)
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const editingId = ref(0)

const query = reactive({ status: '', keyword: '', page: 1, pageSize: 10 })

const form = reactive({
  title: '',
  counterParty: '',
  amount: 0,
  currency: 'CNY',
  signDate: '',
  status: 'draft',
})

const statusMap: Record<string, { label: string; type: 'info' | 'success' | 'warning' | 'danger' }> = {
  draft: { label: '草稿', type: 'info' },
  active: { label: '生效中', type: 'success' },
  expired: { label: '已过期', type: 'warning' },
  terminated: { label: '已终止', type: 'danger' },
}

async function load() {
  loading.value = true
  try {
    const res = await api.contracts({
      status: query.status || undefined,
      keyword: query.keyword || undefined,
      page: query.page,
      pageSize: query.pageSize,
    })
    list.value = res.data?.list || []
    total.value = res.data?.total || 0
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  query.status = ''
  query.keyword = ''
  query.page = 1
  load()
}

function openCreate() {
  editingId.value = 0
  partyTip.value = null
  form.title = ''
  form.counterParty = ''
  form.amount = 0
  form.currency = 'CNY'
  form.signDate = ''
  form.status = 'draft'
  dialogVisible.value = true
}

function openEdit(row: Contract) {
  editingId.value = row.id
  form.title = row.title
  form.counterParty = row.counterParty
  form.amount = row.amount
  form.currency = row.currency || 'CNY'
  form.signDate = row.signDate || ''
  form.status = row.status
  dialogVisible.value = true
}

async function save() {
  if (!form.title.trim()) {
    ElMessage.warning('请填写合同名称')
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await api.updateContract(editingId.value, { ...form })
      ElMessage.success('已更新')
    } else {
      await api.createContract({ ...form })
      ElMessage.success('已创建')
    }
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function remove(row: Contract) {
  const ok = await ElMessageBox.confirm(`确认删除合同「${row.title}」？`, '删除确认', {
    type: 'warning',
    confirmButtonText: '删除',
    cancelButtonText: '取消',
  }).catch(() => false)
  if (!ok) return
  await api.deleteContract(row.id)
  ElMessage.success('已删除')
  await load()
}

function fmtAmount(n: number) {
  return Number(n || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
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
}
.filters {
  display: flex;
  gap: 10px;
  margin-bottom: 14px;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}
.party-tip { margin-top: 8px; line-height: 1.6; }
.party-link { color: #409eff; margin-left: 4px; }
</style>
