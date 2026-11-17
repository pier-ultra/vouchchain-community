<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">发票中心</span>
        <span class="card-sub">US-I1~I8：导入解析 → 查重 → 凭证关联 → 四性检测 → 单套制归档 → 回单匹配 → 验真 → 证据链</span>
      </div>
      <el-upload :show-file-list="false" :auto-upload="false" accept=".ofd,.xml" :on-change="onImportPick">
        <el-button type="primary">
          <el-icon><UploadFilled /></el-icon>
          导入 OFD/XML 发票
        </el-button>
      </el-upload>
      <el-button type="warning" plain @click="redFlushScan">红冲预警扫描</el-button>
    </div>

    <el-tabs v-model="tab">
      <!-- 发票台账 -->
      <el-tab-pane label="发票台账" name="inv">
        <div class="filters">
          <el-input v-model="invQuery.keyword" placeholder="发票号/购销方" clearable style="width: 200px" @keyup.enter="loadInvoices" @clear="loadInvoices" />
          <el-select v-model="invQuery.category" placeholder="分类" clearable style="width: 130px" @change="loadInvoices">
            <el-option v-for="(cnt, cat) in classStats" :key="cat" :label="cat + ' (' + cnt + ')'" :value="cat" />
          </el-select>
          <el-button type="primary" plain @click="loadInvoices">查询</el-button>
        </div>
        <el-table :data="invoices" v-loading="invLoading" border stripe size="small" highlight-current-row @row-click="(row: InvoiceRow) => (selectedInvoice = row)">
          <el-table-column prop="id" label="ID" width="56" />
          <el-table-column prop="invoice_no" label="发票号码" width="150" />
          <el-table-column prop="issue_date" label="开票日期" width="100" />
          <el-table-column prop="buyer_name" label="购买方" min-width="140" show-overflow-tooltip />
          <el-table-column prop="seller_name" label="销售方" min-width="140" show-overflow-tooltip />
          <el-table-column label="价税合计" width="100">
            <template #default="{ row }">¥{{ fmt(row.total) }}</template>
          </el-table-column>
          <el-table-column label="分类" width="100">
            <template #default="{ row }">
              <el-tag v-if="row.category" size="small" effect="plain">{{ row.category }}</el-tag>
              <span v-else style="color: var(--el-text-color-placeholder)">未分类</span>
            </template>
          </el-table-column>
          <el-table-column label="验真" width="90">
            <template #default="{ row }">
              <el-tag v-if="row.verify_status === 'verified'" type="success" size="small">已验真</el-tag>
              <el-tag v-else-if="row.verify_status === 'failed'" type="danger" size="small">验真失败</el-tag>
              <el-tag v-else type="info" size="small">待验真</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="红冲" width="90">
            <template #default="{ row }">
              <el-tag v-if="row.red_flush_status === 'red'" type="danger" size="small">红字</el-tag>
              <el-tag v-else-if="row.red_flush_status === 'reversed'" type="warning" size="small">被红冲</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="归档" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.archive_status === 'archived'" type="success" size="small">已归档</el-tag>
              <el-tag v-else type="info" size="small">未归档</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="凭证" width="80">
            <template #default="{ row }">
              <span v-if="row.voucher_id">#{{ row.voucher_id }}</span>
              <span v-else class="dim">—</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="260" fixed="right">
            <template #default="{ row }">
              <el-button link size="small" type="primary" @click="runIntegrity(row)">四性检测</el-button>
              <el-button link size="small" type="success" :disabled="row.archive_status === 'archived'" @click="archive(row)">归档</el-button>
              <el-button link size="small" :type="row.verify_status === 'verified' ? 'info' : 'warning'" @click="verify(row)">
                {{ row.verify_status === 'verified' ? '取消验真' : '登记验真' }}
              </el-button>
              <el-button link size="small" type="primary" @click="showChain(row)">证据链</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 凭证与回单 -->
      <el-tab-pane label="凭证与回单" name="vr">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-card shadow="never">
              <template #header>
                <div class="blk-head">
                  <span>记账凭证（AI3）</span>
                  <el-button size="small" @click="voucherVisible = true">+ 录入凭证</el-button>
                </div>
              </template>
              <el-table :data="vouchers" border size="small">
                <el-table-column prop="id" label="ID" width="50" />
                <el-table-column prop="voucher_no" label="凭证号" width="100" />
                <el-table-column prop="summary" label="摘要" min-width="140" show-overflow-tooltip />
                <el-table-column label="金额" width="90">
                  <template #default="{ row }">¥{{ fmt(row.amount) }}</template>
                </el-table-column>
                <el-table-column label="操作" width="90">
                  <template #default="{ row }">
                    <el-button link size="small" type="primary" @click="linkVoucher(row)">关联选中发票</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </el-col>
          <el-col :span="12">
            <el-card shadow="never">
              <template #header>
                <div class="blk-head">
                  <span>银行回单（AI6）</span>
                  <div>
                    <el-button size="small" @click="receiptVisible = true">+ 录入回单</el-button>
                    <el-button size="small" type="primary" @click="autoMatch">自动匹配</el-button>
                  </div>
                </div>
              </template>
              <el-table :data="receipts" border size="small">
                <el-table-column prop="id" label="ID" width="50" />
                <el-table-column prop="payer" label="付款方" min-width="110" show-overflow-tooltip />
                <el-table-column label="金额" width="90">
                  <template #default="{ row }">¥{{ fmt(row.amount) }}</template>
                </el-table-column>
                <el-table-column prop="receipt_date" label="日期" width="95" />
                <el-table-column label="匹配" width="110">
                  <template #default="{ row }">
                    <el-tag v-if="row.matched_invoice_id" type="success" size="small">发票#{{ row.matched_invoice_id }}</el-tag>
                    <el-tag v-else type="warning" size="small">待处理</el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </el-col>
        </el-row>
      </el-tab-pane>

      <!-- 证据链（AI8） -->
      <el-tab-pane label="证据链联查" name="chain">
        <div class="filters">
          <el-select v-model="chainInvoiceId" placeholder="选发票" style="width: 280px">
            <el-option v-for="iv in invoices" :key="iv.id" :label="`#${iv.id} ${iv.invoice_no}`" :value="iv.id" />
          </el-select>
          <el-button type="primary" @click="loadChain">一键联查</el-button>
          <el-button v-if="chainInvoiceId" @click="downloadChain('md')">导出报告</el-button>
          <el-button v-if="chainInvoiceId" @click="downloadChain('xlsx')">导出 Excel</el-button>
        </div>
        <el-timeline v-if="chain.length">
          <el-timeline-item
            v-for="(l, i) in chain"
            :key="i"
            :type="l.broken ? 'danger' : 'success'"
            :hollow="l.broken"
          >
            <b>{{ layerName(l.layer) }}</b>
            <el-tag v-if="l.broken" type="danger" size="small" style="margin-left: 6px">断链</el-tag>
            <div class="chain-label">{{ l.label }}</div>
          </el-timeline-item>
        </el-timeline>
        <el-empty v-else description="选择发票后联查" :image-size="80" />
      </el-tab-pane>
    </el-tabs>

    <!-- 录入凭证 -->
    <el-dialog v-model="voucherVisible" title="录入记账凭证" width="440px">
      <el-form label-width="80px">
        <el-form-item label="凭证号"><el-input v-model="voucherForm.voucherNo" /></el-form-item>
        <el-form-item label="摘要"><el-input v-model="voucherForm.summary" /></el-form-item>
        <el-form-item label="金额"><el-input-number v-model="voucherForm.amount" :precision="2" style="width: 180px" /></el-form-item>
        <el-form-item label="日期"><el-input v-model="voucherForm.voucherDate" placeholder="YYYY-MM-DD" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="voucherVisible = false">取消</el-button>
        <el-button type="primary" @click="saveVoucher">保存</el-button>
      </template>
    </el-dialog>

    <!-- 录入回单 -->
    <el-dialog v-model="receiptVisible" title="录入银行回单" width="440px">
      <el-form label-width="80px">
        <el-form-item label="付款方"><el-input v-model="receiptForm.payer" /></el-form-item>
        <el-form-item label="收款方"><el-input v-model="receiptForm.payee" /></el-form-item>
        <el-form-item label="金额"><el-input-number v-model="receiptForm.amount" :precision="2" style="width: 180px" /></el-form-item>
        <el-form-item label="日期"><el-input v-model="receiptForm.receiptDate" placeholder="YYYY-MM-DD" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="receiptVisible = false">取消</el-button>
        <el-button type="primary" @click="saveReceipt">保存</el-button>
      </template>
    </el-dialog>

    <!-- 四性检测报告 -->
    <el-dialog v-model="integrityVisible" title="四性检测报告" width="520px">
      <el-table :data="integrityReport" border size="small">
        <el-table-column prop="name" label="检测项" width="100" />
        <el-table-column label="结果" width="90">
          <template #default="{ row }">
            <el-tag :type="row.pass ? 'success' : 'danger'" size="small">{{ row.pass ? '通过' : '不通过' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="msg" label="说明" min-width="220" />
      </el-table>
    </el-dialog>

    <!-- 证据链抽屉 -->
    <el-drawer v-model="chainVisible" :title="`证据链：发票 #${chainInvoiceId}`" size="50%">
      <el-timeline>
        <el-timeline-item v-for="(l, i) in chain" :key="i" :type="l.broken ? 'danger' : 'success'" :hollow="l.broken">
          <b>{{ layerName(l.layer) }}</b>
          <el-tag v-if="l.broken" type="danger" size="small" style="margin-left: 6px">断链</el-tag>
          <div class="chain-label">{{ l.label }}</div>
        </el-timeline-item>
      </el-timeline>
    </el-drawer>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { UploadFile } from 'element-plus'
import { api, type InvoiceRow, type ReceiptRow, type VoucherRow } from '../api'

const tab = ref('inv')
const invoices = ref<InvoiceRow[]>([])
const invLoading = ref(false)
const invQuery = reactive({ keyword: '', category: '' })
const classStats = ref<Record<string, number>>({})
const selectedInvoice = ref<InvoiceRow | null>(null)

const vouchers = ref<VoucherRow[]>([])
const receipts = ref<ReceiptRow[]>([])

const chainInvoiceId = ref<number | undefined>()
const chain = ref<{ layer: string; id: number; label: string; broken: boolean }[]>([])
const chainVisible = ref(false)
const integrityVisible = ref(false)
const integrityReport = ref<{ name: string; pass: boolean; msg: string }[]>([])

const voucherVisible = ref(false)
const voucherForm = reactive({ voucherNo: '', summary: '', amount: 0, voucherDate: '' })
const receiptVisible = ref(false)
const receiptForm = reactive({ payer: '', payee: '', amount: 0, receiptDate: '', remark: '' })

async function loadInvoices() {
  invLoading.value = true
  try {
    const res = await api.invoices({ keyword: invQuery.keyword || undefined, category: invQuery.category || undefined })
    invoices.value = res.data || []
  } finally {
    invLoading.value = false
  }
}
// A2 红冲预警：全量扫描落库后刷新列表
async function redFlushScan() {
  const res = await api.invoiceRedFlushScan()
  const d = res.data || { red: 0, reversed: 0 }
  ElMessage.success(`红冲预警扫描完成：红字发票 ${d.red} 张，被红冲 ${d.reversed} 张`)
  await loadInvoices()
}
async function loadVr() {
  const [v, r] = await Promise.all([api.vouchers(), api.receipts()])
  vouchers.value = v.data || []
  receipts.value = r.data || []
}

// AI1.1-1.2 / AI2.1-2.4 导入+查重（拦截可人工放行）
async function onImportPick(uploadFile: UploadFile) {
  const raw = uploadFile.raw
  if (!raw) return
  if (!/\.(ofd|xml)$/i.test(raw.name)) {
    ElMessage.error('仅支持 OFD / XML 数电票文件')
    return
  }
  const res: any = await api.invoiceImport(raw)
  if (res.code === 2) {
    // 查重拦截：发票号重复 → 询问人工放行；哈希重复 → 直接跳过
    if (res.data?.duplicate === 'invoiceNo') {
      const ok = await ElMessageBox.confirm(
        `${res.message}。是否人工放行（AI2.4，将留痕）？`,
        '查重拦截',
        { type: 'warning', confirmButtonText: '放行导入', cancelButtonText: '取消' },
      ).catch(() => false)
      if (ok) {
        const r2: any = await api.invoiceImport(raw, true)
        ElMessage.success(`已放行导入（发票 #${r2.data?.id}）`)
      }
    } else {
      ElMessage.info(res.message)
    }
  } else {
    ElMessage.success(`导入成功：发票号 ${res.data?.invoiceNo}，价税合计 ${res.data?.total}`)
  }
  await loadInvoices()
}

// AI4 四性检测
async function runIntegrity(row: InvoiceRow) {
  const res = await api.invoiceIntegrity(row.id)
  integrityReport.value = res.data || []
  integrityVisible.value = true
}

// AI5 归档
async function archive(row: InvoiceRow) {
  await api.invoiceArchive(row.id)
  ElMessage.success('已归档（源件+元数据+检测报告打包）')
  await loadInvoices()
}

// AI7 验真登记（税务接口占位）
async function verify(row: InvoiceRow) {
  const target = row.verify_status === 'verified' ? 'unverified' : 'verified'
  await api.invoiceVerify(row.id, target)
  ElMessage.success(target === 'verified' ? '已登记验真通过（占位，税务接口待接）' : '已撤销验真标记')
  await loadInvoices()
}

// AI3 关联凭证
async function linkVoucher(v: VoucherRow) {
  if (!selectedInvoice.value) {
    ElMessage.warning('请先在台账 tab 中点击要关联的发票行（再次点击选中）')
    return
  }
  await api.invoiceLinkVoucher(selectedInvoice.value.id, v.id)
  ElMessage.success('已关联（留痕）')
  await loadInvoices()
}

// AI6 自动匹配
async function autoMatch() {
  const res = await api.receiptAutoMatch()
  ElMessage.success(`匹配完成：成功 ${res.data?.matched} 笔，待处理 ${res.data?.unmatched} 笔`)
  await loadVr()
}

// AI8 证据链
async function loadChain() {
  if (!chainInvoiceId.value) return
  const res = await api.invoiceChain(chainInvoiceId.value)
  chain.value = res.data?.chain || []
  chainVisible.value = true
}
function showChain(row: InvoiceRow) {
  chainInvoiceId.value = row.id
  loadChain()
}
// AI8.4 联查导出（审计调阅）
function downloadChain(format: 'md' | 'xlsx') {
  if (!chainInvoiceId.value) return
  window.open(api.invoiceChainExportUrl(chainInvoiceId.value, format), '_blank')
}
function layerName(l: string) {
  const m: Record<string, string> = { invoice: '发票', voucher: '记账凭证', receipt: '银行回单' }
  return m[l] || l
}

async function saveVoucher() {
  await api.voucherCreate({ ...voucherForm })
  ElMessage.success('凭证已录入')
  voucherVisible.value = false
  await loadVr()
}
async function saveReceipt() {
  await api.receiptCreate({ ...receiptForm })
  ElMessage.success('回单已录入')
  receiptVisible.value = false
  await loadVr()
}

function fmt(n: number) {
  return Number(n || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2 })
}
function fmtTime(t?: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

onMounted(async () => {
  await loadInvoices()
  await loadVr()
  try {
    const s = await api.http.get<never, any>('/invoice-class/stats')
    classStats.value = s.data || {}
  } catch { /* 分类模块不可用时隐藏筛选项 */ }
})
</script>

<style scoped>
.toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.card-title { font-size: 16px; font-weight: 600; margin-right: 12px; }
.card-sub { font-size: 12px; color: #909399; }
.filters { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; }
.blk-head { display: flex; align-items: center; justify-content: space-between; }
.dim { color: #c0c4cc; }
.chain-label { font-size: 12px; color: #606266; margin-top: 2px; }
</style>
