<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">相对方画像</span>
        <span class="card-sub">US-C8 / AC8.1–AC8.5：按相对方归集合同、履约义务与争议条款，给出签约前风险提示</span>
      </div>
      <el-input v-model="keyword" placeholder="搜索相对方" clearable style="width: 220px" @keyup.enter="loadList" @clear="loadList" />
    </div>

    <el-alert type="info" :closable="false" show-icon class="hint">
      画像只基于本系统内已登记的合同台账、履约义务和条款复核结果，<b>不接外部工商 / 司法征信</b>，
      不是法律意义上的信用评级；样本不足时会明确标注。
    </el-alert>

    <el-row :gutter="12" class="cards">
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">相对方</div><div class="stat-value">{{ summary.parties }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">需重点关注</div><div class="stat-value warn">{{ summary.attention }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">归集合同</div><div class="stat-value">{{ summary.contracts }}</div></el-card></el-col>
      <el-col :span="6"><el-card shadow="never"><div class="stat-label">有履约记录</div><div class="stat-value">{{ summary.withObligation }}</div></el-card></el-col>
    </el-row>

    <el-row :gutter="12">
      <el-col :span="9">
        <el-table :data="list" border stripe highlight-current-row v-loading="loading" height="520" @row-click="select">
          <el-table-column label="相对方" min-width="160">
            <template #default="{ row }">
              <div>{{ row.name }}</div>
              <div class="muted">{{ row.contractCount }} 份合同 · {{ row.lastSignDate || '无签约日' }}</div>
            </template>
          </el-table-column>
          <el-table-column label="标签" min-width="140">
            <template #default="{ row }">
              <el-tag v-for="t in (row.tags || []).slice(0, 2)" :key="t" size="small" class="tag">{{ t }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="风险分" width="82">
            <template #default="{ row }">
              <el-tag :type="levelType(row.riskLevel)" size="small">{{ row.riskScore }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </el-col>
      <el-col :span="15">
        <div v-if="!detail" class="empty">从左侧选一个相对方查看完整画像</div>
        <div v-else v-loading="detailLoading">
          <div class="detail-head">
            <span class="detail-name">{{ detail.profile.counterParty }}</span>
            <el-tag :type="levelType(detail.profile.riskLevel)">{{ detail.riskLabel }}</el-tag>
            <el-tag v-for="t in detail.profile.tags" :key="t" size="small" class="tag">{{ t }}</el-tag>
            <span class="score">风险分 {{ detail.profile.riskScore }} / 100</span>
          </div>
          <el-alert v-if="detail.profile.sampleWarning" type="warning" :closable="false" show-icon class="hint">
            {{ detail.profile.sampleWarning }}
          </el-alert>

          <el-descriptions :column="3" border size="small">
            <el-descriptions-item label="合同数">{{ detail.profile.contractCount }}</el-descriptions-item>
            <el-descriptions-item label="累计金额">¥ {{ fmtAmount(detail.profile.totalAmount) }}</el-descriptions-item>
            <el-descriptions-item label="签约区间">{{ detail.profile.firstSignDate || '—' }} ~ {{ detail.profile.lastSignDate || '—' }}</el-descriptions-item>
            <el-descriptions-item label="已完结义务">{{ detail.profile.obligationDone }} / {{ detail.profile.obligationTotal }}</el-descriptions-item>
            <el-descriptions-item label="按时完成">{{ detail.profile.fulfillRate.toFixed(0) }}%</el-descriptions-item>
            <el-descriptions-item label="逾期未完成">{{ detail.profile.overduePendingCount }}</el-descriptions-item>
            <el-descriptions-item label="争议/违约条款">{{ detail.profile.disputeClauseCount }}</el-descriptions-item>
            <el-descriptions-item label="高风险条款">{{ detail.profile.highRiskClauseCount }}</el-descriptions-item>
            <el-descriptions-item label="驳回/终止合同">{{ detail.profile.rejectedCount }}</el-descriptions-item>
          </el-descriptions>

          <el-tabs v-model="tab" class="detail-tabs">
            <el-tab-pane label="评分依据" name="reasons">
              <ul class="reason-list">
                <li v-for="(r, i) in detail.profile.reasons" :key="i">{{ r }}</li>
              </ul>
              <div class="section-title">争议条款分布</div>
              <el-tag v-for="c in detail.profile.clauseCategories" :key="c.type" class="tag">
                {{ c.type || '未分类' }} × {{ c.count }}
              </el-tag>
              <span v-if="!detail.profile.clauseCategories?.length" class="muted">无争议类条款</span>
            </el-tab-pane>
            <el-tab-pane label="历史合同" name="contracts">
              <el-table :data="detail.profile.contracts" border stripe size="small" max-height="260">
                <el-table-column prop="id" label="ID" width="60" />
                <el-table-column prop="title" label="合同名称" min-width="170" />
                <el-table-column label="金额" width="120">
                  <template #default="{ row }">¥ {{ fmtAmount(row.amount) }}</template>
                </el-table-column>
                <el-table-column prop="status" label="状态" width="100" />
                <el-table-column prop="signDate" label="签订日期" width="110" />
              </el-table>
            </el-tab-pane>
            <el-tab-pane label="关联文件与补绑" name="files">
              <div class="link-row">
                <el-input-number v-model="linkFileId" :min="0" placeholder="合同文件 ID" :controls="false" style="width: 150px" />
                <el-input v-model="linkName" placeholder="相对方名称" style="width: 220px" />
                <el-button v-if="can('party:link')" type="primary" @click="link">绑定</el-button>
              </div>
              <div class="muted">上传时未登记相对方的存量文件，在这里补绑后其义务与条款才会归入画像。</div>
              <el-table :data="files" border stripe size="small" max-height="220">
                <el-table-column prop="id" label="文件ID" width="80" />
                <el-table-column prop="filename" label="文件名" min-width="200" />
                <el-table-column prop="status" label="解析状态" width="110" />
              </el-table>
            </el-tab-pane>
          </el-tabs>
        </div>
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { govApi, type PartyProfileRow } from '../api'
import { can } from '../rbac'

type DetailResp = Awaited<ReturnType<typeof govApi.partyDetail>>['data']

const keyword = ref('')
const tab = ref('reasons')
const loading = ref(false)
const detailLoading = ref(false)
const list = ref<PartyProfileRow[]>([])
const detail = ref<DetailResp | undefined>()
const files = ref<{ id: number; filename: string; counter_party: string; status: string; created_at: string }[]>([])
const summary = reactive({ parties: 0, attention: 0, contracts: 0, withObligation: 0 })
const linkFileId = ref(0)
const linkName = ref('')

function levelType(level: string) {
  return level === 'high' ? 'danger' : level === 'medium' ? 'warning' : 'success'
}
function fmtAmount(v: number) {
  return Number(v || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

async function loadList() {
  loading.value = true
  try {
    const [l, s] = await Promise.all([govApi.partyList(keyword.value || undefined), govApi.partySummary()])
    list.value = l.data || []
    Object.assign(summary, s.data || {})
  } finally {
    loading.value = false
  }
}
async function select(row: PartyProfileRow) {
  detailLoading.value = true
  linkName.value = row.name
  try {
    const [d, f] = await Promise.all([govApi.partyDetail(row.name), govApi.partyFiles(row.name)])
    detail.value = d.data
    files.value = f.data || []
    tab.value = 'reasons'
  } finally {
    detailLoading.value = false
  }
}
async function link() {
  if (!linkFileId.value || !linkName.value.trim()) {
    ElMessage.warning('请填写合同文件 ID 与相对方名称')
    return
  }
  await govApi.partyLink(linkFileId.value, linkName.value.trim())
  ElMessage.success('已绑定，画像即时重算')
  await loadList()
  if (detail.value) await select({ name: linkName.value.trim() } as PartyProfileRow)
}

onMounted(loadList)
</script>

<style scoped>
.toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.card-title { font-size: 16px; font-weight: 600; margin-right: 12px; }
.card-sub { font-size: 12px; color: #909399; }
.hint { margin-bottom: 12px; }
.cards { margin-bottom: 12px; }
.stat-label { font-size: 12px; color: #909399; }
.stat-value { font-size: 24px; font-weight: 600; margin-top: 4px; }
.stat-value.warn { color: #e6a23c; }
.empty { padding: 80px 0; text-align: center; color: #909399; }
.detail-head { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.detail-name { font-size: 18px; font-weight: 600; margin-right: 6px; }
.score { margin-left: auto; font-size: 13px; color: #606266; }
.tag { margin-right: 5px; margin-bottom: 4px; }
.muted { color: #909399; font-size: 12px; }
.section-title { font-weight: 600; margin: 12px 0 8px; }
.reason-list { margin: 4px 0 0; padding-left: 20px; line-height: 1.9; font-size: 13px; }
.detail-tabs { margin-top: 12px; }
.link-row { display: flex; gap: 8px; margin-bottom: 6px; }
</style>
