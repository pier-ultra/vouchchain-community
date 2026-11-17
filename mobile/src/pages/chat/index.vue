<script setup lang="ts">
import { ref, nextTick } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import {
  api,
  chatStream,
  type Row,
  type ChatSessionRow,
  type ChatMessageRow,
  type ChatStep,
} from '../../services/api'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const sessions = ref<ChatSessionRow[]>([])
const currentId = ref<number>(0)
const messages = ref<ChatMessageRow[]>([])
const input = ref('')
const pending = ref(false)
const scrollInto = ref('')
const loadingMsgs = ref(false)

// 流式状态：正在生成的文本 + 工具步骤
const streamText = ref('')
const streamTools = ref<Row[]>([])
let abortStream: (() => void) | null = null

const prompts = [
  '查一下我名下的合同有哪些',
  '本月待审批的申请有多少',
  '帮我总结最近的发票情况',
]

onShow(() => {
  loadSessions()
})

async function loadSessions() {
  try {
    const res = await api.chatSessions()
    if (res?.code === 0) sessions.value = res.data || []
  } catch {
    /* ignore */
  }
}

async function openSession(s: ChatSessionRow) {
  if (currentId.value === s.id) return
  currentId.value = s.id
  loadingMsgs.value = true
  try {
    const res = await api.chatMessages(s.id)
    if (res?.code === 0) messages.value = res.data || []
    else messages.value = []
  } catch {
    messages.value = []
  } finally {
    loadingMsgs.value = false
    scrollBottom()
  }
}

function newSession() {
  currentId.value = 0
  messages.value = []
  input.value = ''
}

function sessionMenu(s: ChatSessionRow) {
  uni.showActionSheet({
    itemList: ['重命名', '删除会话'],
    success: (r) => {
      if (r.tapIndex === 0) renameSession(s)
      else if (r.tapIndex === 1) deleteSession(s)
    },
  })
}

function renameSession(s: ChatSessionRow) {
  uni.showModal({
    title: '重命名会话',
    editable: true,
    content: s.title,
    success: async (r) => {
      if (!r.confirm) return
      const title = (r.content || '').trim()
      if (!title) return
      const res = await api.chatSessionRename(s.id, title)
      if (res?.code === 0) {
        s.title = title
        uni.showToast({ title: '已重命名', icon: 'success' })
      } else {
        uni.showToast({ title: res?.message || '失败', icon: 'none' })
      }
    },
  })
}

function deleteSession(s: ChatSessionRow) {
  uni.showModal({
    title: '删除会话',
    content: `确认删除「${s.title}」？消息一并删除，不可恢复。`,
    success: async (r) => {
      if (!r.confirm) return
      const res = await api.chatSessionDelete(s.id)
      if (res?.code === 0) {
        sessions.value = sessions.value.filter((x) => x.id !== s.id)
        if (currentId.value === s.id) newSession()
        uni.showToast({ title: '已删除', icon: 'success' })
      } else {
        uni.showToast({ title: res?.message || '失败', icon: 'none' })
      }
    },
  })
}

function stepsOf(m: ChatMessageRow): ChatStep[] {
  if (!m.tool_trace) return []
  if (Array.isArray(m.tool_trace)) return m.tool_trace
  try {
    const v = JSON.parse(m.tool_trace as string)
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

function stepLabel(s: ChatStep): string {
  return s.tool || s.action || '工具调用'
}

function hhmm(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}`
}

function scrollBottom() {
  nextTick(() => {
    scrollInto.value = ''
    nextTick(() => { scrollInto.value = 'chat-bottom' })
  })
}

// token 高频，滚动做节流（200ms），done 时强制滚
let lastScroll = 0
function scrollThrottled() {
  const now = Date.now()
  if (now - lastScroll < 200) return
  lastScroll = now
  scrollBottom()
}

function pushAssistant(content: string, steps: ChatStep[] = []) {
  messages.value.push({
    id: Date.now() + 2, role: 'assistant', content,
    tool_trace: steps, created_at: '',
  } as ChatMessageRow)
}

function send(text?: string) {
  const message = (text ?? input.value).trim()
  if (!message || pending.value) return
  input.value = ''
  pending.value = true
  streamText.value = ''
  streamTools.value = []
  messages.value.push({
    id: Date.now(), role: 'user', content: message, created_at: '',
  } as ChatMessageRow)
  scrollBottom()

  let gotStart = false
  let gotToken = false

  abortStream = chatStream(currentId.value || undefined, message, {
    onStart: (sid) => {
      gotStart = true
      if (!currentId.value && sid) {
        currentId.value = sid
        loadSessions()
      }
    },
    onToken: (t) => {
      gotToken = true
      streamText.value += t
      scrollThrottled()
    },
    onTool: (payload) => {
      streamTools.value.push(payload)
      scrollThrottled()
    },
    onDone: (reply, steps, sid) => {
      if (!currentId.value && sid) { currentId.value = sid; loadSessions() }
      pushAssistant(reply || streamText.value, steps)
      streamText.value = ''
      streamTools.value = []
    },
    onError: (msg) => {
      // 未产出任何内容时才降级到非流式，避免重复落库
      if (!gotToken && gotStart) {
        api.chatSend(currentId.value || undefined, message).then((res) => {
          if (res?.code === 0 && res.data) {
            pushAssistant(res.data.reply || '', res.data.steps || [])
          } else {
            pushAssistant('出错：' + (res?.message || msg))
          }
        })
      } else {
        pushAssistant(streamText.value ? streamText.value + '\n\n（生成中断：' + msg + '）' : '出错：' + msg)
      }
      streamText.value = ''
      streamTools.value = []
    },
    onClose: () => {
      pending.value = false
      abortStream = null
      scrollBottom()
    },
  })
}

// 点"思考中"气泡可中断生成
function stopStream() {
  if (abortStream) { abortStream(); abortStream = null }
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <scroll-view class="session-bar" scroll-x>
      <view class="session-track">
        <view class="session new" @click="newSession">＋ 新对话</view>
        <view
          v-for="s in sessions"
          :key="s.id"
          class="session"
          :class="{ active: currentId === s.id }"
          @click="openSession(s)"
          @longpress="sessionMenu(s)"
        >{{ s.title }}</view>
      </view>
    </scroll-view>

    <scroll-view
      class="chat-body"
      scroll-y
      :scroll-into-view="scrollInto"
      :scroll-with-animation="true"
    >
      <view v-if="loadingMsgs" class="center muted">加载中…</view>

      <view v-else-if="!messages.length" class="welcome">
        <view class="welcome-title">VouchChain AI 助手</view>
        <view class="welcome-sub">可以查询合同、审批、发票、义务，也能发起审批</view>
        <view class="prompt-list">
          <view v-for="p in prompts" :key="p" class="prompt" @click="send(p)">{{ p }}</view>
        </view>
      </view>

      <view v-for="m in messages" :key="m.id" class="msg-row" :class="m.role">
        <view class="bubble">
          <view class="content">{{ m.content }}</view>
          <view v-if="stepsOf(m).length" class="steps">
            <view class="step-line" v-for="(s, i) in stepsOf(m)" :key="i">· {{ stepLabel(s) }}</view>
          </view>
          <view v-if="m.created_at" class="time">{{ hhmm(m.created_at) }}</view>
        </view>
      </view>

      <view v-if="pending" class="msg-row assistant">
        <view class="bubble streaming">
          <view v-if="streamTools.length" class="steps">
            <view class="step-line" v-for="(t, i) in streamTools" :key="i">· {{ stepLabel(t) }}</view>
          </view>
          <view class="content">
            {{ streamText || '思考中…' }}<text v-if="streamText" class="cursor">▍</text>
          </view>
        </view>
      </view>
      <view id="chat-bottom" class="bottom-anchor"></view>
    </scroll-view>

    <view class="composer">
      <textarea
        v-model="input"
        class="input"
        placeholder="问我任何合同相关问题…"
        :auto-height="true"
        :maxlength="1000"
        :disabled="pending"
        @confirm="send()"
      />
      <view v-if="pending" class="send stop" @click="stopStream">停止</view>
      <view v-else class="send" :class="{ disabled: !input.trim() }" @click="send()">发送</view>
    </view>
  </view>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--ch-canvas);
}
.session-bar {
  white-space: nowrap;
  background: var(--ch-surface);
  border-bottom: 1rpx solid var(--ch-surface-3);
}
.session-track {
  display: inline-flex;
  padding: 16rpx 20rpx;
  gap: 14rpx;
}
.session {
  display: inline-block;
  padding: 12rpx 22rpx;
  border: 1rpx solid var(--ch-line);
  border-radius: 28rpx;
  font-size: 24rpx;
  color: var(--ch-text);
  max-width: 240rpx;
  overflow: hidden;
  text-overflow: ellipsis;
}
.session.active {
  background: var(--ch-accent);
  border-color: var(--ch-accent);
  color: #fff;
}
.session.new {
  background: var(--ch-accent-soft);
  border-color: var(--ch-accent-soft);
  color: var(--ch-accent);
}
.chat-body {
  flex: 1;
  padding: 24rpx 20rpx;
}
.center {
  text-align: center;
  margin-top: 60rpx;
}
.welcome {
  padding: 60rpx 20rpx;
  text-align: center;
}
.welcome-title {
  font-size: 36rpx;
  font-weight: 500;
  color: var(--ch-ink);
}
.welcome-sub {
  margin-top: 14rpx;
  font-size: 26rpx;
  color: var(--ch-text-muted);
}
.prompt-list {
  margin-top: 48rpx;
}
.prompt {
  background: var(--ch-surface);
  border: 1rpx solid var(--ch-surface-3);
  border-radius: 16rpx;
  padding: 24rpx;
  margin-bottom: 18rpx;
  font-size: 26rpx;
  color: var(--ch-text);
  text-align: left;
}
.msg-row {
  display: flex;
  margin-bottom: 28rpx;
}
.msg-row.user {
  justify-content: flex-end;
}
.msg-row.assistant {
  justify-content: flex-start;
}
.bubble {
  max-width: 82%;
  background: var(--ch-surface);
  border-radius: 20rpx;
  padding: 22rpx 26rpx;
  border: 1rpx solid var(--ch-surface-3);
}
.msg-row.user .bubble {
  background: var(--ch-accent);
  border-color: var(--ch-accent);
  color: #fff;
}
.content {
  font-size: 28rpx;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-all;
}
.steps {
  margin-top: 16rpx;
  padding-top: 14rpx;
  border-top: 1rpx solid var(--ch-surface-3);
}
.step-line {
  font-size: 22rpx;
  color: var(--ch-text-muted);
  line-height: 1.8;
}
.time {
  margin-top: 10rpx;
  font-size: 20rpx;
  color: var(--ch-text-placeholder);
  text-align: right;
}
.msg-row.user .time {
  color: rgba(255, 255, 255, 0.75);
}
.streaming .content {
  font-size: 28rpx;
}
.cursor {
  color: var(--ch-accent);
  animation: blink 1s steps(2, start) infinite;
}
@keyframes blink {
  0%, 50% { opacity: 1; }
  50.01%, 100% { opacity: 0; }
}
.bottom-anchor {
  height: 2rpx;
}
.composer {
  display: flex;
  align-items: flex-end;
  gap: 16rpx;
  background: var(--ch-surface);
  border-top: 1rpx solid var(--ch-surface-3);
  padding: 18rpx 20rpx;
  padding-bottom: calc(18rpx + env(safe-area-inset-bottom));
}
.input {
  flex: 1;
  min-height: 64rpx;
  max-height: 240rpx;
  background: var(--ch-canvas);
  border-radius: 12rpx;
  padding: 16rpx 20rpx;
  font-size: 28rpx;
  line-height: 1.6;
}
.send {
  padding: 0 30rpx;
  height: 64rpx;
  line-height: 64rpx;
  background: var(--ch-accent);
  color: #fff;
  border-radius: 12rpx;
  font-size: 28rpx;
}
.send.disabled {
  background: var(--ch-text-placeholder);
}
.send.stop {
  background: var(--ch-danger);
}
.muted {
  color: var(--ch-text-muted);
  font-size: 26rpx;
}
</style>
