<template>
  <div>
    <el-tabs v-model="tab" type="border-card">
      <!-- 报销切片 -->
      <el-tab-pane label="报销切片" name="split">
        <div class="toolbar-line">
          <el-input-number v-model="splitForm.invoiceId" :min="1" placeholder="发票 ID" style="width: 130px" />
          <el-button @click="loadSplits">载入发票切片</el-button>
          <el-tag v-if="invoiceTotal > 0" :type="remaining < 0 ? 'danger' : 'info'" effect="plain">
            总额 {{ invoiceTotal.toFixed(2) }} / 已切 {{ splitSum.toFixed(2) }} / 可切 {{ remaining.toFixed(2) }}
          </el-tag>
        </div>
        <el-table :data="splits" size="small" border v-loading="splitLoading">
          <el-table-column prop="sliceNo" label="片" width="50" />
          <el-table-column prop="amount" label="金额" width="110" />
          <el-table-column prop="expenseType" label="用途" width="110" />
          <el-table-column prop="deptId" label="部门ID" width="80" />
          <el-table-column label="承担" width="90">
            <template #default="{ row }">{{ row.bearer === 'personal' ? '个人垫付' : '公司' }}</template>
          </el-table-column>
          <el-table-column prop="remark" label="备注" min-width="140" show-overflow-tooltip />
          <el-table-column prop="creator" label="创建人" width="90" />
          <el-table-column label="操作" width="80" fixed="right">
            <template #default="{ row }">
              <el-button link type="danger" size="small" @click="delSplit(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-divider content-position="left">新增切片</el-divider>
        <div class="toolbar-line">
          <el-input-number v-model="newSplit.amount" :min="0.01" :precision="2" placeholder="金额" style="width: 140px" />
          <el-select v-model="newSplit.expenseType" placeholder="用途" style="width: 140px" allow-create filterable>
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
          <el-input-number v-model="newSplit.deptId" :min="0" placeholder="部门ID" style="width: 120px" />
          <el-select v-model="newSplit.bearer" style="width: 120px">
            <el-option label="公司承担" value="company" />
            <el-option label="个人垫付" value="personal" />
          </el-select>
          <el-input v-model="newSplit.remark" placeholder="备注" style="width: 180px" />
          <el-button type="primary" :loading="saving" @click="addSplit">切</el-button>
        </div>
        <el-alert v-for="(w, i) in splitWarnings" :key="i" :title="w" type="warning" :closable="true" style="margin-top: 8px" />
      </el-tab-pane>

      <!-- 费控预算 -->
      <el-tab-pane label="费控预算" name="budget">
        <div class="toolbar-line">
          <el-select v-model="newBudget.deptId" placeholder="部门" style="width: 150px">
            <el-option label="部门 1" :value="1" />
            <el-option label="部门 2" :value="2" />
          </el-select>
          <el-date-picker v-model="budgetMonth" type="month" value-format="YYYY-MM" placeholder="月份" style="width: 130px" />
          <el-select v-model="newBudget.category" placeholder="分类" style="width: 140px" allow-create filterable>
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
          <el-input-number v-model="newBudget.amountLimit" :min="1" :precision="2" placeholder="上限" style="width: 140px" />
          <el-button type="primary" @click="addBudget">设置预算</el-button>
        </div>
        <el-table :data="budgets" size="small" border v-loading="budgetLoading">
          <el-table-column prop="period" label="月份" width="90" />
          <el-table-column prop="deptId" label="部门ID" width="80" />
          <el-table-column prop="category" label="分类" width="110" />
          <el-table-column prop="limit" label="上限" width="110" />
          <el-table-column prop="spent" label="已用" width="110" />
          <el-table-column label="使用度" min-width="180">
            <template #default="{ row }">
              <el-progress :percentage="pct(row)" :status="row.over ? 'exception' : (row.spent / row.limit > 0.9 ? 'warning' : 'success')" />
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.over" type="danger" size="small">超支</el-tag>
              <el-tag v-else type="success" size="small">正常</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="80" fixed="right">
            <template #default="{ row }"><el-button link type="danger" size="small" @click="delBudget(row)">删除</el-button></template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 差旅标准 -->
      <el-tab-pane label="差旅标准" name="standard">
        <div class="toolbar-line">
          <el-select v-model="newStd.category" placeholder="分类" style="width: 140px" allow-create filterable>
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
          <el-input-number v-model="newStd.limitAmount" :min="1" :precision="2" placeholder="单笔上限" style="width: 140px" />
          <el-input v-model="newStd.note" placeholder="说明（如：一线城市住宿）" style="width: 200px" />
          <el-button type="primary" @click="addStd">设置标准</el-button>
        </div>
        <el-table :data="standards" size="small" border v-loading="stdLoading">
          <el-table-column prop="category" label="分类" width="120" />
          <el-table-column prop="limitAmount" label="单笔上限" width="120" />
          <el-table-column prop="note" label="说明" min-width="200" />
          <el-table-column label="启用" width="80">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '启用' : '停用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="80" fixed="right">
            <template #default="{ row }"><el-button link type="danger" size="small" @click="delStd(row)">删除</el-button></template>
          </el-table-column>
        </el-table>
        <el-divider content-position="left">单发票超标检查</el-divider>
        <div class="toolbar-line">
          <el-input-number v-model="checkInvoiceId" :min="1" placeholder="发票 ID" style="width: 130px" />
          <el-button type="warning" :loading="checking" @click="check">跑超标检查</el-button>
        </div>
        <el-alert v-for="(w, i) in checkWarnings" :key="i" :title="w" type="warning" :closable="true" style="margin-top: 8px" />
        <el-alert v-if="checkDone && checkWarnings.length === 0" title="未发现超标项" type="success" :closable="true" style="margin-top: 8px" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const categories = ['设备采购', '咨询服务', '软件服务', '办公用品', '差旅交通', '餐饮招待', '广告宣传', '其他']
const tab = ref('split')

// 切片
const splits = ref<any[]>([])
const splitSum = ref(0)
const invoiceTotal = ref(0)
const splitLoading = ref(false)
const saving = ref(false)
const splitWarnings = ref<string[]>([])
const splitForm = ref({ invoiceId: 1 })
const newSplit = ref({ amount: 100, expenseType: '差旅交通', deptId: 1, bearer: 'company', remark: '' })

async function loadSplits() {
  splitLoading.value = true
  try {
    const res = await api.http.get<never, any>(`/expense-splits?invoiceId=${splitForm.value.invoiceId}`)
    splits.value = res.data?.list || []
    splitSum.value = res.data?.sum || 0
    invoiceTotal.value = res.data?.invoiceTotal || 0
  } finally {
    splitLoading.value = false
  }
}
const remaining = ref(0)
import { watch } from 'vue'
watch([splitSum, invoiceTotal], () => { remaining.value = invoiceTotal.value - splitSum.value })

async function addSplit() {
  saving.value = true
  try {
    const res = await api.http.post<never, any>('/expense-splits', {
      invoiceId: splitForm.value.invoiceId, ...newSplit.value,
    })
    splitWarnings.value = res.data?.warnings || []
    if (splitWarnings.value.length) ElMessage.warning('切片已保存，但有提示，见下方')
    else ElMessage.success('切片已保存')
    loadSplits()
  } finally {
    saving.value = false
  }
}
async function delSplit(row: any) {
  await api.http.delete<never, any>(`/expense-splits/${row.id}`)
  ElMessage.success('已删除')
  loadSplits()
}

// 预算
const budgets = ref<any[]>([])
const budgetLoading = ref(false)
const budgetMonth = ref(new Date().toISOString().slice(0, 7))
const newBudget = ref({ deptId: 1, category: '差旅交通', amountLimit: 5000 })
async function loadBudgets() {
  budgetLoading.value = true
  try {
    const res = await api.http.get<never, any>('/budgets/usage')
    budgets.value = res.data || []
  } finally {
    budgetLoading.value = false
  }
}
async function addBudget() {
  await api.http.post<never, any>('/budgets', { ...newBudget.value, period: budgetMonth.value })
  ElMessage.success('预算已设置')
  loadBudgets()
}
async function delBudget(row: any) {
  await api.http.delete<never, any>(`/budgets/${row.id}`)
  ElMessage.success('已删除')
  loadBudgets()
}
function pct(row: any): number {
  if (!row.limit) return 0
  return Math.min(100, Math.round((row.spent / row.limit) * 100))
}

// 差标
const standards = ref<any[]>([])
const stdLoading = ref(false)
const newStd = ref({ category: '差旅交通', limitAmount: 500, note: '' })
const checkInvoiceId = ref(1)
const checkWarnings = ref<string[]>([])
const checkDone = ref(false)
const checking = ref(false)
async function loadStds() {
  stdLoading.value = true
  try {
    const res = await api.http.get<never, any>('/travel-standards')
    standards.value = res.data || []
  } finally {
    stdLoading.value = false
  }
}
async function addStd() {
  await api.http.post<never, any>('/travel-standards', { ...newStd.value })
  ElMessage.success('标准已设置')
  loadStds()
}
async function delStd(row: any) {
  await api.http.delete<never, any>(`/travel-standards/${row.id}`)
  ElMessage.success('已删除')
  loadStds()
}
async function check() {
  checking.value = true
  checkDone.value = false
  try {
    const res = await api.http.post<never, any>('/travel-check', { invoiceId: checkInvoiceId.value })
    checkWarnings.value = res.data?.warnings || []
    checkDone.value = true
  } finally {
    checking.value = false
  }
}

onMounted(() => { loadSplits(); loadBudgets(); loadStds() })
</script>

<style scoped>
.toolbar-line { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; flex-wrap: wrap; }
</style>
