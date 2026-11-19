<script setup lang="ts">
import { ref, nextTick, onUnmounted } from 'vue'
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

// ========== 语音识别 (STT) ==========
const recognizing = ref(false)
let recognition: any = null
let recognitionTimer: any = null

// 兼容 H5 / 微信小程序 / App 的语音识别初始化
function initSpeechRecognition() {
  // #ifdef H5
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
        input.value = transcript
      }

      recognition.onerror = (event: any) => {
        console.warn('[STT] 识别错误:', event.error)
        if (event.error === 'no-speech' || event.error === 'audio-capture') {
          // 静默重试
          setTimeout(() => { if (recognizing.value) recognition.start() }, 300)
        } else if (event.error === 'not-allowed') {
          uni.showToast({ title: '请授权麦克风权限', icon: 'none' })
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
  // #endif
}

// 开始语音输入
function startSTT() {
  if (recognizing.value) return

  // #ifdef H5
  if (!recognition) initSpeechRecognition()
  if (recognition) {
    try {
      recognition.start()
      recognizing.value = true
      uni.vibrateShort({ type: 'medium' })
    } catch (e) {
      uni.showToast({ title: '启动语音识别失败', icon: 'none' })
    }
  } else {
    uni.showToast({ title: '当前浏览器不支持语音识别', icon: 'none' })
  }
  // #endif

  // #ifdef MP-WEIXIN || APP-PLUS
  // 微信小程序/App 使用原生录音识别
  const plugin = requirePlugin('speechRecognition') || null
  if (plugin) {
    // 微信小程序插件方式（需在 app.json 配置插件）
    // 这里简化：直接用 uni 的录音+云识别（需云函数）
    uni.showToast({ title: '小程序端请长按语音键', icon: 'none' })
  } else {
    // App 端可用 plus.speech
    // #ifdef APP-PLUS
    const plus = (window as any).plus
    if (plus && plus.speech) {
      const recognizer = plus.speech.createRecognizer('ifly', { engine: 'iFly', language: 'zh_cn' })
      recognizer.onrecognition = (result: any) => {
        if (result.text) input.value = result.text
      }
      recognizer.onerror = (e: any) => console.warn('[STT] App 识别错误:', e)
      recognizer.start()
      recognizing.value = true
      // 存到全局以便停止
      ;(window as any).__stt_recognizer = recognizer
      uni.vibrateShort({ type: 'medium' })
    } else {
      uni.showToast({ title: '请在 H5/浏览器端使用语音输入', icon: 'none' })
    }
    // #endif
    // #ifndef APP-PLUS
    uni.showToast({ title: '请在 H5/浏览器端使用语音输入', icon: 'none' })
    // #endif
  }
  // #endif
}

// 停止语音输入
function stopSTT() {
  if (!recognizing.value) return
  recognizing.value = false

  // #ifdef H5
  if (recognition) {
    try { recognition.stop() } catch { }
  }
  // #endif

  // #ifdef APP-PLUS
  const recognizer = (window as any).__stt_recognizer
  if (recognizer) {
    try { recognizer.stop() } catch { }
    ;(window as any).__stt_recognizer = null
  }
  // #endif

  uni.vibrateShort({ type: 'light' })
}

// ========== 语音合成 (TTS) ==========
let speechSynthesis: any = null
let currentUtterance: any = null
const speakingMsgId = ref<number | null>(null)

// 初始化 TTS
function initSpeechSynthesis() {
  // #ifdef H5
  if (!speechSynthesis) {
    speechSynthesis = (window as any).speechSynthesis
  }
  // #endif

  // #ifdef APP-PLUS
  // App 端可用 plus.speech
  // #endif
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

  // #ifdef H5
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
  // #endif

  // #ifdef APP-PLUS
  const plus = (window as any).plus
  if (plus && plus.speech) {
    speakingMsgId.value = msgId
    plus.speech.speak({ text, voice: 'xiaoyan', language: 'zh_cn' }, () => {
      speakingMsgId.value = null
    }, (e: any) => {
      console.warn('[TTS] App 播报错误:', e)
      speakingMsgId.value = null
    })
  }
  // #endif

  // #ifdef MP-WEIXIN
  // 微信小程序：调用云函数 TTS 或使用内置组件（简化提示）
  uni.showToast({ title: '小程序端暂不支持语音播报', icon: 'none' })
  // #endif
}

// 停止朗读
function stopSpeak() {
  // #ifdef H5
  if (speechSynthesis) {
    try { speechSynthesis.cancel() } catch { }
  }
  currentUtterance = null
  // #endif

  // #ifdef APP-PLUS
  const plus = (window as any).plus
  if (plus && plus.speech) {
    try { plus.speech.stopSpeak() } catch { }
  }
  // #endif

  speakingMsgId.value = null
}

// 页面卸载清理
onUnmounted(() => {
  stopSTT()
  stopSpeak()
})

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
          <view class="bubble-header">
            <view class="content">{{ m.content }}</view>
            <view v-if="m.role === 'assistant'" class="bubble-actions">
              <view class="action-btn" :class="{ playing: speakingMsgId === m.id }" @click="speakText(m.content, m.id)" @longpress="stopSpeak">
                <text>{{ speakingMsgId === m.id ? '⏹️' : '🔊' }}</text>
              </view>
            </view>
          </view>
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
      <view class="input-wrap">
        <view class="voice-btn" :class="{ recording: recognizing }" @click="recognizing ? stopSTT() : startSTT()">
          <text>{{ recognizing ? '⏹️' : '🎤' }}</text>
        </view>
        <textarea
          v-model="input"
          class="input"
          placeholder="问我任何合同相关问题…"
          :auto-height="true"
          :maxlength="1000"
          :disabled="pending || recognizing"
          @confirm="send()"
        />
      </view>
      <view v-if="pending" class="send stop" @click="stopStream">停止</view>
      <view v-else-if="recognizing" class="send stop" @click="stopSTT">停止录音</view>
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

/* ===== 语音相关样式 ===== */
.composer {
  align-items: center; /* 改为垂直居中，方便语音按钮对齐 */
}
.input-wrap {
  display: flex;
  align-items: flex-end;
  gap: 12rpx;
  flex: 1;
}
.voice-btn {
  width: 64rpx;
  height: 64rpx;
  border-radius: 12rpx;
  background: var(--ch-surface-2);
  border: 1rpx solid var(--ch-line);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28rpx;
  color: var(--ch-text);
  flex-shrink: 0;
  margin-bottom: 4rpx;
}
.voice-btn.recording {
  background: var(--ch-danger-soft);
  border-color: var(--ch-danger);
  color: var(--ch-danger);
  animation: pulse 1s infinite;
}
@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.05); opacity: 0.8; }
}

.bubble-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8rpx;
}
.bubble-actions {
  display: flex;
  gap: 8rpx;
  flex-shrink: 0;
  margin-top: 4rpx;
}
.action-btn {
  width: 48rpx;
  height: 48rpx;
  border-radius: 10rpx;
  background: var(--ch-surface-2);
  border: 1rpx solid var(--ch-line);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22rpx;
  color: var(--ch-text-muted);
}
.action-btn.playing {
  background: var(--ch-accent-soft);
  border-color: var(--ch-accent);
  color: var(--ch-accent);
}
.action-btn:active {
  transform: scale(0.95);
}

/* 流式气泡也支持语音播报（生成完成后） */
.msg-row.assistant .bubble.streaming .bubble-actions {
  display: none; /* 生成中隐藏播报按钮 */
}
</style>
