<template>
  <el-container class="chat-page">
    <!-- 左：会话列表（增删改查） -->
    <el-aside width="250px" class="session-aside">
      <el-button type="primary" style="width: 100%" :icon="Plus" @click="newSession">新建对话</el-button>
      <div class="session-list">
        <div
          v-for="s in sessions"
          :key="s.id"
          class="session-item"
          :class="{ active: s.id === currentId }"
          @click="switchSession(s.id)"
        >
          <span class="session-title">{{ s.title }}</span>
          <span class="session-ops">
            <el-icon title="重命名" @click.stop="renameSession(s)"><Edit /></el-icon>
            <el-icon title="删除" @click.stop="removeSession(s)"><Delete /></el-icon>
          </span>
        </div>
        <div v-if="!sessions.length" class="session-empty">还没有对话</div>
      </div>
    </el-aside>

    <!-- 右：对话主区 -->
    <el-main class="chat-main">
      <div class="chat-toolbar">
        <div class="toolbar-left">
          <span class="title-dot"></span>
          <span class="chat-title">{{ currentTitle || '新对话' }}</span>
        </div>
        <el-tooltip :disabled="!mcp?.enabled" placement="top">
          <template #content>
            外部 AI 客户端（Claude Desktop / Cursor 等）可通过 MCP 调用本系统 {{ mcp?.tools }} 个工具：<br />
            端点 {{ mcp?.endpoint }}<br />
            请求头 {{ JSON.stringify(mcp?.headers) }}
          </template>
          <el-tag size="small" :type="mcp?.enabled ? 'success' : 'info'" effect="plain">
            MCP {{ mcp?.enabled ? `已启用 · ${mcp?.tools} 工具` : '未启用' }}
          </el-tag>
        </el-tooltip>
      </div>

      <div class="chat-body" ref="bodyRef">
        <div v-if="!messages.length && !pending" class="chat-welcome">
          <div class="welcome-icon">
            <el-icon :size="26"><ChatDotRound /></el-icon>
          </div>
          <div class="welcome-eyebrow">CONTRACT INTELLIGENCE</div>
          <div class="welcome-title">你好，有什么可以帮你？</div>
          <div class="welcome-sub">用自然语言查询合同、识别风险、发起审批<br />每一步操作都留痕可溯</div>
          <div class="welcome-cards">
            <div class="welcome-card" @click="draft = '查一下和华东建筑的合同'">
              <span class="welcome-card-icon"><el-icon><Search /></el-icon></span>
              <span class="welcome-card-text">
                <strong>查一下和华东建筑的合同</strong>
                <small>自然语言检索，自动查库</small>
              </span>
              <el-icon class="welcome-card-arrow"><ArrowRight /></el-icon>
            </div>
            <div class="welcome-card" @click="draft = '华东建筑靠不靠谱？'">
              <span class="welcome-card-icon"><el-icon><TrendCharts /></el-icon></span>
              <span class="welcome-card-text">
                <strong>华东建筑靠不靠谱？</strong>
                <small>相对方风险画像与历史履约</small>
              </span>
              <el-icon class="welcome-card-arrow"><ArrowRight /></el-icon>
            </div>
            <div class="welcome-card" @click="draft = '把XX合同发起审批'">
              <span class="welcome-card-icon"><el-icon><Stamp /></el-icon></span>
              <span class="welcome-card-text">
                <strong>把XX合同发起审批</strong>
                <small>直接发起并完整留痕</small>
              </span>
              <el-icon class="welcome-card-arrow"><ArrowRight /></el-icon>
            </div>
          </div>
          <div class="welcome-note">数据边界：仅本系统内部数据，不含外部工商/司法</div>
        </div>
        <div v-for="m in messages" :key="m.id" class="msg-row" :class="m.role">
          <div v-if="m.role === 'assistant'" class="msg-avatar ai"><el-icon :size="14"><ChatDotRound /></el-icon></div>
          <div class="msg-stack">
            <div v-if="m.role === 'assistant' && stepsOf(m).length" class="msg-steps">
              <div v-for="(s, i) in stepsOf(m)" :key="i" class="step-line">
                <el-icon :color="s.ok ? '#0f9d6e' : '#d64545'"><component :is="s.ok ? 'CircleCheck' : 'CircleClose'" /></el-icon>
                <span class="step-tool">{{ toolLabel(s.tool) }}</span>
                <span class="step-summary">{{ s.summary }}</span>
              </div>
            </div>
            <div class="bubble">
              {{ m.content }}
              <div v-if="m.role === 'assistant'" class="bubble-actions">
                <button class="action-btn" :class="{ playing: speakingMsgId === m.id }" @click="speakText(m.content, m.id)" @contextmenu.prevent="stopSpeak" :aria-label="speakingMsgId === m.id ? '停止播报' : '语音播报'">
                  <Monitor v-if="speakingMsgId !== m.id" />
                  <Stopwatch v-else />
                </button>
              </div>
            </div>
          </div>
          <div v-if="m.role === 'user'" class="msg-avatar me">我</div>
        </div>
        <div v-if="pending" class="msg-row assistant">
          <div class="msg-avatar ai"><el-icon :size="14"><ChatDotRound /></el-icon></div>
          <div class="msg-stack">
            <div class="bubble pending"><span class="typing-dots"><i></i><i></i><i></i></span> 正在思考与执行工具…</div>
          </div>
        </div>
      </div>

      <div class="chat-input">
        <div class="input-wrap">
          <button class="voice-btn" :class="{ recording: recognizing }" @click="recognizing ? stopSTT() : startSTT()" :disabled="pending" :aria-label="recognizing ? '停止语音输入' : '开始语音输入'">
            <Mic v-if="!recognizing" />
            <Stopwatch v-else />
          </button>
          <el-input
            v-model="draft"
            type="textarea"
            :rows="3"
            resize="none"
            placeholder="用一句话说要查什么 / 做什么，Enter 发送，Shift+Enter 换行"
            @keydown.enter.exact.prevent="send"
            :disabled="pending || recognizing"
          />
        </div>
        <el-button type="primary" :loading="pending" @click="send">发送</el-button>
      </div>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import { Plus, Edit, Delete, Mic, Monitor, Stopwatch } from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { api, type ChatSessionRow, type ChatMessageRow, type ChatStep, type McpStatus } from '../api'

const sessions = ref<ChatSessionRow[]>([])
const currentId = ref<number>(0)
const messages = ref<ChatMessageRow[]>([])
const draft = ref('')
const pending = ref(false)
const bodyRef = ref<HTMLElement | null>(null)
const mcp = ref<McpStatus | null>(null)

// ========== 语音识别 (STT) ==========
const recognizing = ref(false)
let recognition: any = null

// 兼容 H5 / 微信小程序 / App 的语音识别初始化
function initSpeechRecognition() {
  if (!recognition) {
    const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition
    if (SpeechRecognition) {
      recognition = new SpeechRecognition()
      recognition.lang = 'zh-CN'
      recognition.continuous = true
      recognition.interimResults = true
      recognition.maxAlternatives = 1

      recognition.onresult = (event: any) => {
        let transcript = ''
        for (let i = event.resultIndex; i < event.results.length; i++) {
          transcript += event.results[i][0].transcript
        }
        draft.value = transcript
      }

      recognition.onerror = (event: any) => {
        console.warn('[STT] 识别错误:', event.error)
        if (event.error === 'no-speech' || event.error === 'audio-capture') {
          // 静默重试
          setTimeout(() => { if (recognizing.value) recognition.start() }, 300)
        } else if (event.error === 'not-allowed') {
          ElMessage.warning('请授权麦克风权限')
          stopSTT()
        }
      }

      recognition.onend = () => {
        if (recognizing.value) {
          // 连续识别：自动重启
          setTimeout(() => { if (recognizing.value) recognition.start() }, 100)
        }
      }
    }
  }
}

// 开始语音输入
function startSTT() {
  if (recognizing.value) return

  if (!recognition) initSpeechRecognition()
  if (recognition) {
    try {
      recognition.start()
      recognizing.value = true
    } catch (e) {
      ElMessage.error('启动语音识别失败')
    }
  } else {
    ElMessage.error('当前浏览器不支持语音识别')
  }
}

// 停止语音输入
function stopSTT() {
  if (!recognizing.value) return
  recognizing.value = false

  if (recognition) {
    try { recognition.stop() } catch { }
  }
}

// ========== 语音合成 (TTS) ==========
let speechSynthesis: any = null
let currentUtterance: any = null
const speakingMsgId = ref<number | null>(null)

// 初始化 TTS
function initSpeechSynthesis() {
  if (!speechSynthesis) {
    speechSynthesis = (window as any).speechSynthesis
  }
}

// 朗读文本
function speakText(text: string, msgId: number) {
  if (!text.trim()) return

  // 如果正在朗读同一条，则停止
  if (speakingMsgId.value === msgId) {
    stopSpeak()
    return
  }

  stopSpeak() // 停止其它正在播放的

  if (speechSynthesis) {
    const utter = new SpeechSynthesisUtterance(text)
    utter.lang = 'zh-CN'
    utter.rate = 1.0
    utter.pitch = 1.0
    utter.volume = 1.0

    utter.onstart = () => { speakingMsgId.value = msgId }
    utter.onend = () => { speakingMsgId.value = null }
    utter.onerror = () => { speakingMsgId.value = null }

    currentUtterance = utter
    speechSynthesis.speak(utter)
  }
}

// 停止朗读
function stopSpeak() {
  if (speechSynthesis) {
    try { speechSynthesis.cancel() } catch { }
  }
  currentUtterance = null
  speakingMsgId.value = null
}

// 页面卸载清理
onUnmounted(() => {
  stopSTT()
  stopSpeak()
})

const currentTitle = computed(() => sessions.value.find(s => s.id === currentId.value)?.title || '')

const toolLabels: Record<string, string> = {
  query_contracts: '查询合同',
  get_contract: '查合同详情',
  get_party_profile: '相对方画像',
  query_invoices: '查询发票',
  query_obligations: '查询义务',
  query_approvals: '查询审批',
  start_contract_approval: '发起审批',
  act_approval: '审批操作',
}
function toolLabel(t: string) {
  return toolLabels[t] || t
}

function stepsOf(m: ChatMessageRow): ChatStep[] {
  try {
    return m.tool_trace ? JSON.parse(m.tool_trace) : []
  } catch {
    return []
  }
}

async function loadSessions() {
  try {
    const res = await api.chatSessions()
    sessions.value = res.data || []
  } catch { /* 拦截器已提示 */ }
}

async function loadMcp() {
  try {
    const res = await api.mcpStatus()
    mcp.value = res.data || null
  } catch { mcp.value = null }
}

async function switchSession(id: number) {
  currentId.value = id
  try {
    const res = await api.chatMessages(id)
    messages.value = res.data || []
    scrollBottom()
  } catch { /* 拦截器已提示 */ }
}

function newSession() {
  currentId.value = 0
  messages.value = []
}

async function renameSession(s: ChatSessionRow) {
  try {
    const { value } = await ElMessageBox.prompt('新的对话标题', '重命名', {
      inputValue: s.title, inputPattern: /\S/, inputErrorMessage: '标题不能为空',
    })
    await api.chatSessionRename(s.id, value.trim())
    s.title = value.trim()
    ElMessage.success('已重命名')
  } catch { /* 取消或失败静默 */ }
}

async function removeSession(s: ChatSessionRow) {
  try {
    await ElMessageBox.confirm(`删除对话「${s.title}」？消息将一并清除。`, '删除对话', { type: 'warning' })
  } catch {
    return
  }
  await api.chatSessionDelete(s.id)
  if (currentId.value === s.id) {
    currentId.value = 0
    messages.value = []
  }
  loadSessions()
  ElMessage.success('已删除')
}

async function send() {
  const text = draft.value.trim()
  if (!text || pending.value) return
  draft.value = ''
  pending.value = true
  messages.value.push({ id: Date.now(), role: 'user', content: text, tool_trace: '', created_at: '' } as ChatMessageRow)
  scrollBottom()

  // 流式优先：SSE 逐 token 渲染；失败/超时回退同步接口
  const streamingMsg = reactive({ id: Date.now() + 9, role: 'assistant', content: '', tool_trace: '', created_at: '' } as ChatMessageRow)
  let streamed = false
  const sseCtrl = new AbortController()
  const SSE_TIMEOUT_MS = 60000 // 60s 内未完成流式则中止并回退同步，避免前端永久 pending
  const sseStart = Date.now()
  try {
    const token = localStorage.getItem('ch_token') || ''
    const resp = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token, 'X-User-Role': localStorage.getItem('ch_role') || '', 'X-User-Name': localStorage.getItem('ch_user') || '' },
      body: JSON.stringify({ sessionId: currentId.value || undefined, message: text }),
      signal: sseCtrl.signal,
    })
    if (resp.ok && resp.headers.get('content-type')?.includes('text/event-stream')) {
      streamed = true
      messages.value.push(streamingMsg)
      scrollBottom()
      const reader = resp.body!.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      let eventName = 'message'
      outer: for (;;) {
        // 流式读取防挂起：每次 read 最多等 5s，无数据则检查总超时
        // （SSE 长连接在某些浏览器/网络下可能只推首帧后静默，需超时兜底回退同步接口）
        const r = await Promise.race([
          reader.read(),
          new Promise((res) => setTimeout(() => res({ done: true, __timeout: true }), 5000)),
        ]) as { done: boolean; value?: Uint8Array; __timeout?: boolean }
        if (r.__timeout) {
          if (Date.now() - sseStart > SSE_TIMEOUT_MS) {
            sseCtrl.abort()
            streamed = false
            const idx = messages.value.indexOf(streamingMsg)
            if (idx >= 0) messages.value.splice(idx, 1)
            break
          }
          continue
        }
        const { done, value } = r
        if (done) break
        buf += decoder.decode(value, { stream: true })
        // 解析 SSE 帧（event/data 对）
        for (;;) {
          const idx = buf.indexOf('\n\n')
          if (idx < 0) break
          const frame = buf.slice(0, idx)
          buf = buf.slice(idx + 2)
          let data = ''
          eventName = 'message'
          for (const line of frame.split('\n')) {
            if (line.startsWith('event: ')) eventName = line.slice(7).trim()
            else if (line.startsWith('data: ')) data += line.slice(6)
          }
          if (!data) continue
          if (eventName === 'start') {
            const d = JSON.parse(data)
            currentId.value = d.sessionId
          } else if (eventName === 'tool') {
            const d = JSON.parse(data)
            streamingMsg.tool_trace = JSON.stringify([d])
            scrollBottom()
          } else if (eventName === 'token') {
            const d = JSON.parse(data)
            streamingMsg.content += d.t
            scrollBottom()
          } else if (eventName === 'done') {
            const d = JSON.parse(data)
            streamingMsg.content = d.reply
            streamingMsg.tool_trace = JSON.stringify(d.steps || [])
            break outer
          } else if (eventName === 'error') {
            const d = JSON.parse(data)
            streamingMsg.content = streamingMsg.content || ('出错：' + d.message)
            break outer
          }
        }
      }
      loadSessions()
    }
  } catch { /* 流式失败走回退 */ }

  if (!streamed) {
    try {
      const res = await api.chatSend({ sessionId: currentId.value || undefined, message: text })
      if (res.code === 2) {
        messages.value.push({ id: Date.now() + 1, role: 'assistant', content: res.message || '未配置 AI 模型', tool_trace: '', created_at: '' } as ChatMessageRow)
      } else if (res.data) {
        currentId.value = res.data.sessionId
        messages.value.push({
          id: Date.now() + 2, role: 'assistant', content: res.data.reply,
          tool_trace: JSON.stringify(res.data.steps || []), created_at: '',
        } as ChatMessageRow)
        loadSessions()
      }
    } catch { /* 拦截器已提示；本地保留用户消息便于重试 */ }
  }
  pending.value = false
  scrollBottom()
}

function scrollBottom() {
  nextTick(() => {
    if (bodyRef.value) bodyRef.value.scrollTop = bodyRef.value.scrollHeight
  })
}

onMounted(async () => {
  initSpeechSynthesis()
  loadMcp()
  await loadSessions()
  // 进页面默认接最近会话，接着上次聊
  if (sessions.value.length) switchSession(sessions.value[0].id)
})
</script>

<style scoped>
.chat-page {
  height: calc(100vh - 140px);
  margin: 16px;
  border: 1px solid var(--ch-line-soft);
  border-radius: 16px;
  background: var(--ch-surface);
  box-shadow: var(--el-box-shadow-light);
  overflow: hidden;
}

/* ---- 左：会话列表 ---- */
.session-aside {
  border-right: 1px solid var(--ch-line-soft);
  background: var(--ch-surface-2);
  padding: 16px 12px;
  display: flex; flex-direction: column; gap: 14px;
}
.session-aside :deep(.el-button--primary) {
  border-radius: 8px;
  height: 40px;
  font-size: 13px;
  font-weight: 600;
  box-shadow: 0 2px 8px rgba(79, 70, 229, 0.16);
}
.session-list { flex: 1; overflow-y: auto; margin: 0 -6px; padding: 0 6px; }
.session-item {
  display: flex; align-items: center; justify-content: space-between;
  padding: 11px 12px; border-radius: 8px; cursor: pointer; margin-bottom: 3px;
  font-size: 13px; color: var(--el-text-color-regular);
  border: 1px solid transparent;
  transition: background-color 0.16s, color 0.16s, border-color 0.16s;
}
.session-item:hover { background: var(--ch-surface-3); }
.session-item.active {
  background: var(--ch-surface);
  color: var(--ch-accent);
  border-color: var(--ch-line);
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(16, 24, 40, 0.05);
}
html.dark .session-item.active { box-shadow: 0 1px 4px rgba(0, 0, 0, 0.35); }
.session-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
.session-ops { display: none; gap: 8px; }
.session-item:hover .session-ops { display: inline-flex; }
.session-ops .el-icon:hover { color: var(--ch-accent); }
.session-empty { color: var(--el-text-color-placeholder); font-size: 12px; text-align: center; margin-top: 24px; }

/* ---- 右：主区 ---- */
.chat-main { display: flex; flex-direction: column; padding: 0; background: #fff; min-width: 0; }
.chat-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0 24px; height: 64px;
  border-bottom: 1px solid var(--ch-line-soft);
  background: var(--ch-surface);
  flex: 0 0 auto;
}
.toolbar-left { display: flex; align-items: center; gap: 10px; min-width: 0; }
.title-dot {
  width: 7px; height: 7px; border-radius: 50%; flex: 0 0 auto;
  background: var(--ch-accent);
  box-shadow: 0 0 0 4px var(--ch-accent-soft);
}
.chat-title { font-weight: 650; font-size: 14px; letter-spacing: -0.01em; color: var(--el-text-color-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* ---- 消息区 ---- */
.chat-body {
  flex: 1; min-height: 0; overflow-y: auto; padding: 28px 32px;
  background: var(--ch-surface-2);
}
.chat-welcome {
  max-width: 620px;
  margin: clamp(32px, 8vh, 88px) auto 0;
  text-align: center;
  color: var(--el-text-color-secondary);
}
.welcome-icon {
  width: 52px; height: 52px; margin: 0 auto 20px;
  display: flex; align-items: center; justify-content: center;
  border-radius: 14px; color: var(--ch-accent);
  background: var(--ch-surface);
  border: 1px solid var(--ch-line);
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04), 0 8px 24px rgba(79, 70, 229, 0.06);
}
.welcome-eyebrow { font-size: 10px; font-weight: 700; letter-spacing: 0.18em; color: var(--ch-accent); opacity: 0.75; }
.welcome-title {
  font-size: 24px; font-weight: 700; letter-spacing: -0.03em;
  margin: 9px 0 8px; color: var(--el-text-color-primary);
}
.welcome-sub { font-size: 13px; line-height: 1.8; color: var(--el-text-color-secondary); }
.welcome-cards { display: grid; grid-template-columns: 1fr; gap: 10px; margin: 30px auto 0; max-width: 480px; text-align: left; }
.welcome-card {
  display: flex; align-items: center; gap: 13px;
  padding: 13px 15px;
  background: var(--ch-surface); border: 1px solid var(--ch-line-soft);
  border-radius: 10px; cursor: pointer; color: var(--el-text-color-regular);
  transition: border-color 0.16s, box-shadow 0.16s, transform 0.16s;
}
.welcome-card-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: 36px; height: 36px; flex: 0 0 auto;
  border-radius: 9px; background: var(--ch-accent-soft); color: var(--ch-accent);
  font-size: 16px;
}
.welcome-card-text { display: flex; flex-direction: column; gap: 1px; min-width: 0; flex: 1; }
.welcome-card-text strong { font-size: 13px; font-weight: 600; color: var(--el-text-color-primary); }
.welcome-card-text small { font-size: 11px; color: var(--el-text-color-placeholder); }
.welcome-card-arrow { color: var(--el-text-color-disabled); font-size: 14px; transition: transform 0.16s, color 0.16s; }
.welcome-card:hover { border-color: var(--el-color-primary-light-7); box-shadow: 0 4px 16px rgba(16, 24, 40, 0.06); transform: translateY(-1px); }
.welcome-card:hover .welcome-card-arrow { color: var(--ch-accent); transform: translateX(3px); }
.welcome-note {
  margin-top: 26px; font-size: 11px; color: var(--el-text-color-placeholder);
  padding-top: 14px; border-top: 1px solid var(--ch-line-soft);
}
.msg-row { display: flex; align-items: flex-start; gap: 10px; margin-bottom: 22px; }
.msg-row.user { flex-direction: row-reverse; }
.msg-avatar {
  width: 30px; height: 30px; flex: 0 0 auto;
  display: flex; align-items: center; justify-content: center;
  border-radius: 9px; font-size: 11px; font-weight: 700;
  margin-top: 2px;
}
.msg-avatar.ai { background: var(--ch-accent-soft); color: var(--ch-accent); border: 1px solid var(--el-color-primary-light-8); }
.msg-avatar.me { background: var(--ch-user-bubble); color: var(--ch-user-bubble-text); border-radius: 50%; }
.msg-stack { display: flex; flex-direction: column; max-width: min(76%, 760px); min-width: 0; }
.msg-row.user .msg-stack { align-items: flex-end; }
.bubble {
  padding: 10px 14px; border-radius: 12px; font-size: 13.5px;
  line-height: 1.8; white-space: pre-wrap; word-break: break-word;
  letter-spacing: 0.005em;
}
.msg-row.user .bubble {
  background: var(--ch-user-bubble); color: var(--ch-user-bubble-text);
  border-bottom-right-radius: 3px;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.12);
}
.msg-row.assistant .bubble {
  background: var(--ch-surface); color: var(--el-text-color-primary);
  border: 1px solid var(--ch-line-soft);
  border-bottom-left-radius: 3px;
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.03);
}
.bubble.pending {
  color: var(--el-text-color-secondary); background: var(--ch-surface);
  border-style: dashed; display: flex; align-items: center; gap: 7px;
}
.typing-dots { display: inline-flex; gap: 3px; align-items: center; }
.typing-dots i {
  width: 4px; height: 4px; border-radius: 50%; background: var(--ch-accent);
  animation: ch-blink 1.2s infinite ease-in-out;
}
.typing-dots i:nth-child(2) { animation-delay: 0.18s; }
.typing-dots i:nth-child(3) { animation-delay: 0.36s; }
@keyframes ch-blink {
  0%, 60%, 100% { opacity: 0.25; transform: translateY(0); }
  30% { opacity: 1; transform: translateY(-2px); }
}
.msg-steps {
  margin-bottom: 7px; padding: 8px 12px;
  background: var(--ch-surface-3); border: 1px solid var(--ch-line-soft);
  border-radius: 8px;
}
.step-line { display: flex; align-items: center; gap: 7px; font-size: 11.5px; color: var(--el-text-color-secondary); line-height: 22px; }
.step-tool { font-weight: 600; color: var(--ch-accent); }
.step-summary { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* ---- 输入区 ---- */
.chat-input {
  display: flex; gap: 12px; align-items: flex-end;
  padding: 16px 24px 18px;
  border-top: 1px solid var(--ch-line-soft);
  background: var(--ch-surface);
  flex: 0 0 auto;
}
.chat-input :deep(.el-textarea__inner) {
  border-radius: 10px;
  padding: 11px 13px;
  line-height: 1.7;
  font-size: 13.5px;
  box-shadow: 0 0 0 1px var(--ch-line) inset;
  transition: box-shadow 0.16s;
}
.chat-input :deep(.el-textarea__inner:focus) {
  box-shadow: 0 0 0 1px var(--ch-accent) inset, 0 0 0 3px var(--ch-accent-soft) !important;
}
.chat-input :deep(.el-button--primary) {
  height: 40px; padding: 0 22px;
  border-radius: 8px; font-weight: 600; font-size: 13px;
}

/* ===== 语音相关样式 ===== */
.input-wrap {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  flex: 1;
}
.voice-btn {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: var(--ch-surface-2);
  border: 1px solid var(--ch-line);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ch-text);
  flex-shrink: 0;
  margin-bottom: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.voice-btn:hover:not(:disabled) {
  background: var(--ch-accent-soft);
  border-color: var(--ch-accent);
  color: var(--ch-accent);
}
.voice-btn.recording {
  background: #fef0f0;
  border-color: #f56c6c;
  color: #f56c6c;
  animation: pulse 1s infinite;
}
.voice-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.05); opacity: 0.8; }
}

.bubble-actions {
  display: flex;
  gap: 6px;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--ch-line-soft);
}
.action-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--ch-surface-2);
  border: 1px solid var(--ch-line);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);
  cursor: pointer;
  transition: all 0.16s;
}
.action-btn:hover {
  background: var(--ch-accent-soft);
  border-color: var(--ch-accent);
  color: var(--ch-accent);
}
.action-btn.playing {
  background: var(--ch-accent-soft);
  border-color: var(--ch-accent);
  color: var(--ch-accent);
}
.action-btn:active {
  transform: scale(0.95);
}
</style>
