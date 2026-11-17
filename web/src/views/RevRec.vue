<template>
  <div>
    <el-tabs v-model="tab" type="border-card">
      <!-- 确认操作 -->
      <el-tab-pane label="确认操作" name="op">
        <div class="toolbar-line">
          <el-input-number v-model="op.obligationId" :min="1" placeholder="义务 ID" style="width: 130px" />
          <el-select v-model="op.mode" style="width: 170px">
            <el-option label="时点法（完成即确认）" value="point" />
            <el-option label="时段法（投入法）" value="time" />
          </el-select>
          <el-input-number v-model="op.shareRatio" :min="0" :max="100" :precision="1" placeholder="分摊比例%" style="width: 130px" />
          <el-input-number v-model="op.taxRate" :min="0" :max="20" :precision="1" placeholder="税率%" style="width: 110px" />
          <el-input-number v-if="op.mode === 'time'" v-model="op.plannedCost" :min="0" :precision="2" placeholder="预计总成本" style="width: 150px" />
          <el-button @click="setMode">保存确认方式</el-button>
        </div>
        <div class="toolbar-line">
          <template v-if="op.mode === 'time'">
            <el-input-number v-model="op.accumCost" :min="0" :precision="2" placeholder="本期累计成本" style="width: 150px" />
          </template>
          <el-button type="primary" :loading="suggesting" @click="suggest">生成确认建议</el-button>
        </div>
        <el-alert v-for="(w, i) in warnings" :key="i" :title="w" type="warning" :closable="true" style="margin-bottom: 6px" />
        <el-divider content-position="left">确认记录（draft → 人审确认）</el-divider>
        <el-table :data="records" size="small" border v-loading="loading">
          <el-table-column prop="id" label="ID" width="56" />
          <el-table-column prop="obligationId" label="义务" width="70" />
          <el-table-column prop="period" label="期间" width="80" />
          <el-table-column prop="amount" label="确认收入(不含税)" width="130" />
          <el-table-column prop="tax" label="销项税" width="100" />
          <el-table-column prop="total" label="价税合计" width="110" />
          <el-table-column prop="progress" label="进度%" width="80" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'confirmed' ? 'success' : 'warning'" size="small">
                {{ row.status === 'confirmed' ? '已确认' : '待确认' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button v-if="row.status === 'draft'" link type="primary" size="small" @click="confirmRow(row)">确认入账</el-button>
              <span v-else class="hint">草稿 #{{ row.draftId }}</span>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 合同结算台账 v2：确认/结算/收款三线 + 合同资产/负债标注 -->
      <el-tab-pane label="合同结算台账" name="ledger">
        <div style="margin-bottom: 8px">
          <el-button type="primary" size="small" @click="exportRevRec">导出审计口径 Excel</el-button>
          <span class="hint" style="margin-left: 8px">台账 / 确认明细 / 应收预收差异 / 月度三线 四张表合一文件，供对账与审计取证。</span>
        </div>
        <el-table :data="ledger" size="small" border v-loading="ledgerLoading">
          <el-table-column prop="contractFileId" label="合同文件" width="90" />
          <el-table-column prop="title" label="标题" min-width="180" show-overflow-tooltip />
          <el-table-column prop="contractAmount" label="合同金额" width="105" />
          <el-table-column prop="confirmed" label="已确认(含税)" width="120" />
          <el-table-column prop="settled" label="已结算" width="105" />
          <el-table-column prop="received" label="已收款" width="105" />
          <el-table-column label="差额(确认−结算)" width="150">
            <template #default="{ row }">
              <el-tag :type="row.balanceType === 'asset' ? 'warning' : row.balanceType === 'liability' ? 'info' : 'success'" size="small">
                {{ row.balanceType === 'asset' ? '合同资产' : row.balanceType === 'liability' ? '合同负债' : '平衡' }}
                {{ row.balance > 0 ? row.balance.toFixed(2) : '' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="未回款(应收)" width="110">
            <template #default="{ row }">
              <span :style="row.unreceived > 0.005 ? 'color: var(--el-color-danger); font-weight: 600' : ''">
                {{ row.unreceived.toFixed(2) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="openSettle(row)">登记结算</el-button>
              <el-button link type="primary" size="small" @click="openReceipt(row)">登记回款</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 应收实收差异（R-A7）：应收带账龄 vs 预收 -->
      <el-tab-pane label="应收/实收差异" name="arcr">
        <el-table :data="arcr" size="small" border v-loading="arcrLoading">
          <el-table-column prop="contractFileId" label="合同文件" width="90" />
          <el-table-column prop="title" label="标题" min-width="180" show-overflow-tooltip />
          <el-table-column prop="confirmed" label="已确认(含税)" width="120" />
          <el-table-column prop="received" label="已收款" width="110" />
          <el-table-column label="应收(确认未收)" width="140">
            <template #default="{ row }">
              <span :style="row.receivable > 0.005 ? 'color: var(--el-color-danger); font-weight: 600' : ''">{{ row.receivable.toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="预收(收未确认)" width="140">
            <template #default="{ row }">
              <span :style="row.advance > 0.005 ? 'color: var(--el-color-primary); font-weight: 600' : ''">{{ row.advance.toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="应收账龄(天)" width="110">
            <template #default="{ row }">
              <span v-if="row.receivable > 0.005" :style="row.agingDays > 90 ? 'color: var(--el-color-danger)' : ''">{{ row.agingDays }}</span>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 月度视图（R-A11）：确认/结算/收款三条线 -->
      <el-tab-pane label="月度视图" name="monthly">
        <el-table :data="monthly" size="small" border v-loading="monthlyLoading">
          <el-table-column prop="period" label="期间" width="110" />
          <el-table-column prop="confirmed" label="确认收入(含税)" width="150" />
          <el-table-column prop="settled" label="已结算" width="150" />
          <el-table-column prop="received" label="已收款" width="150" />
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="receiptVisible" title="登记回款" width="420px">
      <el-form label-width="90px">
        <el-form-item label="金额" required><el-input-number v-model="receipt.amount" :min="0.01" :precision="2" style="width: 100%" /></el-form-item>
        <el-form-item label="到账日" required><el-date-picker v-model="receipt.receivedAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="receipt.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="receiptVisible = false">取消</el-button>
        <el-button type="primary" @click="saveReceipt">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settleVisible" title="登记结算（出结算单）" width="420px">
      <el-form label-width="90px">
        <el-form-item label="金额" required><el-input-number v-model="settle.amount" :min="0.01" :precision="2" style="width: 100%" /></el-form-item>
        <el-form-item label="结算日" required><el-date-picker v-model="settle.settledAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="settle.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settleVisible = false">取消</el-button>
        <el-button type="primary" @click="saveSettle">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const tab = ref('op')
const op = ref({ obligationId: 1, mode: 'point', shareRatio: 100, taxRate: 13, plannedCost: 0, accumCost: 0 })
const records = ref<any[]>([])
const warnings = ref<string[]>([])
const loading = ref(false)
const suggesting = ref(false)
const ledger = ref<any[]>([])
const ledgerLoading = ref(false)
const arcr = ref<any[]>([])
const arcrLoading = ref(false)
const monthly = ref<any[]>([])
const monthlyLoading = ref(false)
const receiptVisible = ref(false)
const receipt = ref({ contractFileId: 0, amount: 0, receivedAt: new Date().toISOString().slice(0, 10), remark: '' })
const settleVisible = ref(false)
const settle = ref({ contractFileId: 0, amount: 0, settledAt: new Date().toISOString().slice(0, 10), remark: '' })

async function loadRecords() {
  loading.value = true
  try {
    const res = await api.http.get<never, any>('/revrec/list')
    records.value = res.data || []
  } finally { loading.value = false }
}

async function loadLedger() {
  ledgerLoading.value = true
  try {
    const res = await api.http.get<never, any>('/revrec/ledger/v2')
    ledger.value = res.data || []
  } finally { ledgerLoading.value = false }
}

async function loadArCr() {
  arcrLoading.value = true
  try {
    const res = await api.http.get<never, any>('/revrec/ar-cr-diff')
    arcr.value = res.data || []
  } finally { arcrLoading.value = false }
}

async function loadMonthly() {
  monthlyLoading.value = true
  try {
    const res = await api.http.get<never, any>('/revrec/monthly')
    monthly.value = res.data || []
  } finally { monthlyLoading.value = false }
}

async function setMode() {
  await api.http.put<never, any>(`/revrec/mode/${op.value.obligationId}`, {
    mode: op.value.mode, shareRatio: op.value.shareRatio,
    taxRate: op.value.taxRate, plannedCost: op.value.plannedCost,
  })
  ElMessage.success('确认方式已保存')
}

async function suggest() {
  suggesting.value = true
  warnings.value = []
  try {
    const res = await api.http.post<never, any>('/revrec/suggest', {
      obligationId: op.value.obligationId, accumCost: op.value.accumCost,
    })
    warnings.value = res.data?.warnings || []
    if (warnings.value.length) ElMessage.warning('已生成，但有提示')
    else ElMessage.success('确认建议已生成（待人审确认）')
    loadRecords()
  } finally { suggesting.value = false }
}

async function confirmRow(row: any) {
  const res = await api.http.post<never, any>(`/revrec/confirm/${row.id}`, {})
  ElMessage.success(`已确认，分录草稿 #${res.data?.draftId}`)
  loadRecords(); loadLedger(); loadMonthly()
}

function openReceipt(row: any) {
  receipt.value = { contractFileId: row.contractFileId, amount: row.unreceived > 0 ? row.unreceived : 0, receivedAt: new Date().toISOString().slice(0, 10), remark: '' }
  receiptVisible.value = true
}

async function saveReceipt() {
  await api.http.post<never, any>('/revrec/receipt', receipt.value)
  ElMessage.success('回款已登记')
  receiptVisible.value = false
  loadLedger(); loadArCr(); loadMonthly()
}

function openSettle(row: any) {
  settle.value = { contractFileId: row.contractFileId, amount: row.contractAmount > 0 ? row.contractAmount : 0, settledAt: new Date().toISOString().slice(0, 10), remark: '' }
  settleVisible.value = true
}

async function saveSettle() {
  await api.http.post<never, any>('/revrec/settlement', settle.value)
  ElMessage.success('结算已登记')
  settleVisible.value = false
  loadLedger(); loadArCr(); loadMonthly()
}

// R-A14 审计口径 Excel 导出（服务端生成，四张表合一文件）
function exportRevRec() {
  window.open('/api/revrec/export', '_blank')
}

onMounted(() => { loadRecords(); loadLedger(); loadArCr(); loadMonthly() })
</script>

<style scoped>
.toolbar-line { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; flex-wrap: wrap; }
.hint { font-size: 12px; color: #909399; }
</style>
