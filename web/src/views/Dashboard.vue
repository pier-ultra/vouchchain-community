<template>
  <div>
    <!-- 顶部问候：每日工作台 -->
    <el-card shadow="never" class="hero">
      <div class="hero-hello">你好，{{ userName }}</div>
      <div class="hero-sub">今天是 {{ todayStr }}。已为你聚合本周关注、进行中追踪与下一步建议。</div>
    </el-card>

    <!-- 本周关注卡（角标数字，点击直达） -->
    <div class="sec-title">本周关注</div>
    <el-row :gutter="16" class="mb">
      <el-col v-for="c in cards" :key="c.key" :span="colSpan">
        <el-card shadow="never" class="kpi" @click="go(c.url)">
          <div class="kpi-num" :style="{ color: c.tone }">{{ c.count }}</div>
          <div class="kpi-cap">{{ c.label }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 进行中追踪 + Agent 洞察 -->
    <el-row :gutter="16" class="mb">
      <el-col :span="14">
        <el-card shadow="never">
          <template #header><b>进行中追踪</b><span class="hint">（审批到哪一环 / 发票验真 / 合同状态）</span></template>
          <div v-for="(t, i) in tracks" :key="i" class="track" @click="go(t.url)">
            <div class="track-title">{{ t.title }}</div>
            <div class="track-progress"><el-tag size="small" :type="trackTone(t.kind)">{{ t.progress }}</el-tag></div>
          </div>
          <el-empty v-if="!tracks.length" :image-size="50" description="暂无进行中事项" />
        </el-card>
      </el-col>
      <el-col :span="10">
        <el-card shadow="never">
          <template #header><b>Agent 洞察</b><span class="hint">（下一步建议）</span></template>
          <div v-for="(ins, i) in insights" :key="i" class="insight" :class="ins.level">
            <span class="dot" :class="ins.level"></span>
            <span class="ins-text">{{ ins.text }}</span>
          </div>
          <el-empty v-if="!insights.length" :image-size="50" description="暂无建议" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 晨间简报 / 晚间总结（定时推送 + 手动生成） -->
    <el-card shadow="never" class="mb">
      <template #header>
        <div class="brief-head">
          <b>晨间简报 / 晚间总结</b>
          <span class="hint">每日 08:00 / 21:00 自动生成并推送到在线页面（WebSocket），也可手动生成</span>
          <span>
            <el-button size="small" :loading="genLoading" @click="genBrief('morning')">生成晨报</el-button>
            <el-button size="small" type="primary" :loading="genLoading" @click="genBrief('evening')">生成晚间总结</el-button>
          </span>
        </div>
      </template>
      <div v-if="lastBrief" class="brief-body">
        <div class="brief-meta">{{ lastBrief.kind === 'morning' ? '晨间简报' : '晚间总结' }} · {{ lastBrief.at }}</div>
        <pre class="brief-text">{{ lastBrief.text }}</pre>
      </div>
      <el-empty v-else :image-size="50" description="尚未生成。每日 08:00 / 21:00 自动生成并推送，也可点击上方按钮手动生成。" />
    </el-card>

    <!-- 图表区：收入确认月度三线 + 督办状态 -->
    <el-row :gutter="16">
      <el-col :span="14">
        <el-card shadow="never">
          <template #header><b>收入确认 · 月度三线（确认/结算/收款）</b></template>
          <div ref="monthlyEl" class="chart"></div>
          <el-empty v-if="!monthly.length" :image-size="60" description="暂无收入确认数据" />
        </el-card>
      </el-col>
      <el-col :span="10">
        <el-card shadow="never">
          <template #header><b>公文督办 · 状态分布</b></template>
          <div ref="dubanEl" class="chart"></div>
          <el-empty v-if="!duban.total" :image-size="60" description="暂无督办任务" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, computed, nextTick } from 'vue'
import * as echarts from 'echarts'
import { ElMessage, ElNotification } from 'element-plus'
import { api } from '../api'
import { useTheme } from '../composables/useTheme'

const { isDark } = useTheme()
const textColor = () => (isDark.value ? '#c8c9cc' : '#606266')

// ---------- 每日工作台聚合 ----------
const cards = ref<{ key: string; label: string; count: number; tone: string; url: string }[]>([])
const tracks = ref<{ kind: string; title: string; status: string; progress: string; url: string }[]>([])
const insights = ref<{ level: string; text: string }[]>([])
const lastBrief = ref<{ kind: string; text: string; at: string } | null>(null)
const genLoading = ref(false)
const userName = localStorage.getItem('ch_user') || '同事'
const todayStr = new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })
const colSpan = computed(() => Math.max(4, Math.floor(24 / Math.max(cards.value.length, 1))))

function go(url: string) { location.hash = '#' + url }
function trackTone(kind: string) { return kind === 'invoice' ? 'warning' : kind === 'contract' ? 'success' : 'primary' }

async function loadDaily() {
  try {
    const res = await api.dailyOverview()
    if (res?.code === 0) {
      const o = res.data?.overview || {}
      cards.value = o.cards || []
      tracks.value = o.tracks || []
      insights.value = o.insights || []
      lastBrief.value = res.data?.lastBrief || null
    }
  } catch { /* 兜底已由拦截器提示 */ }
}

async function genBrief(kind: 'morning' | 'evening') {
  genLoading.value = true
  try {
    const res = await api.dailyBrief(kind)
    if (res?.code === 0) {
      lastBrief.value = { kind: res.data?.kind || kind, text: res.data?.text || '', at: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) }
      ElMessage.success('已生成' + (kind === 'morning' ? '晨间简报' : '晚间总结'))
    }
  } finally { genLoading.value = false }
}

// ---------- WebSocket 主动推送（晨/晚总结实时可达） ----------
let ws: WebSocket | null = null
let wsTimer: ReturnType<typeof setInterval> | null = null
function connectWs() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const token = localStorage.getItem('ch_token') || ''
  const query = token ? `?token=${encodeURIComponent(token)}` : ''
  try {
    ws = new WebSocket(`${proto}://${location.host}/api/ws/tasks${query}`)
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data)
        if (msg.type === 'daily_brief') {
          const d = msg.data || {}
          lastBrief.value = { kind: d.kind, text: d.text, at: d.at }
          ElNotification({ title: d.kind === 'morning' ? '晨间简报' : '晚间总结', message: String(d.text || '').slice(0, 90), type: 'info', duration: 6000 })
        }
      } catch { /* ignore */ }
    }
    ws.onclose = () => { ws = null }
  } catch { /* 断线由定时重连兜底 */ }
}
function startWsReconnect() {
  connectWs()
  wsTimer = setInterval(() => { if (!ws) connectWs() }, 30000)
}

// ---------- 图表（保留：收入确认月度 + 督办环形） ----------
interface MonthlyLine { period: string; confirmed: number; settled: number; received: number }
const monthly = ref<MonthlyLine[]>([])
const duban = ref<{ total: number; todo: number; overdue: number; done: number }>({ total: 0, todo: 0, overdue: 0, done: 0 })
const monthlyEl = ref<HTMLElement>()
const dubanEl = ref<HTMLElement>()
let monthlyChart: echarts.ECharts | null = null
let dubanChart: echarts.ECharts | null = null

async function loadMonthly() {
  const res = await api.http.get<any>('/revrec/monthly')
  monthly.value = (res?.data || []).map((r: any) => ({
    period: r.period || r.Period || '', confirmed: Number(r.confirmed ?? r.Confirmed ?? 0),
    settled: Number(r.settled ?? r.Settled ?? 0), received: Number(r.received ?? r.Received ?? 0),
  }))
}
async function loadDuban() {
  const res = await api.http.get<any>('/duban/stats')
  const d = res?.data || {}
  duban.value = { total: d.total ?? 0, todo: d.todo ?? 0, overdue: d.overdue ?? 0, done: d.done ?? 0 }
}

function renderCharts() {
  if (monthlyEl.value && !monthlyChart) monthlyChart = echarts.init(monthlyEl.value)
  if (dubanEl.value && !dubanChart) dubanChart = echarts.init(dubanEl.value)
  if (monthlyChart && monthly.value.length) {
    monthlyChart.setOption({
      textStyle: { color: textColor() }, tooltip: { trigger: 'axis' },
      legend: { data: ['确认', '结算', '收款'], textStyle: { color: textColor() } },
      grid: { left: 50, right: 16, top: 36, bottom: 28 },
      xAxis: { type: 'category', data: monthly.value.map((m) => m.period), axisLabel: { color: textColor() } },
      yAxis: { type: 'value', axisLabel: { color: textColor() } },
      series: [
        { name: '确认', type: 'line', smooth: true, data: monthly.value.map((m) => m.confirmed) },
        { name: '结算', type: 'line', smooth: true, data: monthly.value.map((m) => m.settled) },
        { name: '收款', type: 'line', smooth: true, data: monthly.value.map((m) => m.received) },
      ],
    })
  }
  if (dubanChart && duban.value.total) {
    dubanChart.setOption({
      textStyle: { color: textColor() }, tooltip: { trigger: 'item' },
      legend: { bottom: 0, textStyle: { color: textColor() } },
      series: [{
        type: 'pie', radius: ['42%', '68%'], center: ['50%', '44%'],
        data: [
          { name: '待办/进行中', value: duban.value.todo, itemStyle: { color: '#4f46e5' } },
          { name: '逾期', value: duban.value.overdue, itemStyle: { color: '#d64545' } },
          { name: '已完成', value: duban.value.done, itemStyle: { color: '#0f9d6e' } },
        ], label: { color: textColor() },
      }],
    })
  }
}

onMounted(async () => {
  await Promise.allSettled([loadDaily(), loadMonthly(), loadDuban()])
  await nextTick()
  renderCharts()
  startWsReconnect()
  window.addEventListener('resize', onResize)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  if (wsTimer) clearInterval(wsTimer)
  ws?.close()
  monthlyChart?.dispose(); dubanChart?.dispose()
})
function onResize() { monthlyChart?.resize(); dubanChart?.resize() }
</script>

<style scoped>
.hero { background: linear-gradient(135deg, var(--ch-accent, #4f46e5) 0%, var(--ch-accent-strong, #3730a3) 100%); color: #fff; margin-bottom: 16px; border: none; }
.hero :deep(.el-card__body) { padding: 20px 24px; }
.hero-hello { font-size: 20px; font-weight: 700; }
.hero-sub { margin-top: 8px; font-size: 13px; opacity: 0.9; }
.sec-title { font-size: 15px; font-weight: 600; margin: 4px 0 12px; color: var(--el-text-color-primary, #303133); }
.mb { margin-bottom: 16px; }
.kpi { cursor: pointer; }
.kpi-num { font-size: 30px; font-weight: 700; line-height: 1.1; }
.kpi-cap { margin-top: 8px; font-size: 13px; color: #909399; }
.hint { font-size: 12px; color: #909399; font-weight: 400; margin-left: 8px; }
.track { display: flex; align-items: center; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid var(--el-border-color-lighter, #ebeef5); cursor: pointer; }
.track:last-child { border-bottom: none; }
.track-title { font-size: 14px; color: var(--el-text-color-primary, #303133); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.track-progress { flex-shrink: 0; margin-left: 12px; }
.insight { display: flex; align-items: flex-start; gap: 8px; padding: 10px 0; border-bottom: 1px solid var(--el-border-color-lighter, #ebeef5); }
.insight:last-child { border-bottom: none; }
.dot { width: 8px; height: 8px; border-radius: 50%; margin-top: 6px; flex-shrink: 0; }
.dot.info { background: #4f46e5; } .dot.warn { background: #c77c0a; } .dot.danger { background: #d64545; }
.ins-text { font-size: 13px; line-height: 1.6; color: var(--el-text-color-regular, #606266); }
.brief-head { display: flex; align-items: center; justify-content: space-between; }
.brief-body { white-space: pre-wrap; }
.brief-meta { font-size: 12px; color: #909399; margin-bottom: 8px; }
.brief-text { font-size: 13px; line-height: 1.8; color: var(--el-text-color-regular, #606266); margin: 0; font-family: inherit; }
.chart { width: 100%; height: 320px; }
</style>
