<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">智能问答</span>
        <span class="card-sub">AC5：自然语言查合同库（哪些合同 30 天内到期 / 某主体报价对比…），回答强制标注原文引用</span>
      </div>
    </div>

    <div class="ask-bar">
      <el-input
        v-model="question"
        placeholder="例如：哪些合同的付款截止日在 2026 年 12 月之前？逾期未完成的义务有哪些？"
        @keyup.enter="ask"
      />
      <el-button type="primary" :loading="loading" @click="ask">
        <el-icon><ChatDotRound /></el-icon>
        提问
      </el-button>
    </div>

    <div v-if="history.length" class="history">
      <el-card v-for="(h, i) in history" :key="i" shadow="never" class="qa-card">
        <div class="q"><el-icon><User /></el-icon> {{ h.question }}</div>
        <div class="a md-body" v-html="h.html"></div>
        <div v-if="h.refs.length" class="refs">
          <div class="refs-title">引用来源（{{ h.refs.length }}）</div>
          <div v-for="(r, j) in h.refs" :key="j" class="ref-item">
            <el-tag size="small" type="primary">#{{ r.fileId }} {{ r.filename }}</el-tag>
            <blockquote class="ref-quote">{{ r.quote }}</blockquote>
          </div>
        </div>
        <div v-else class="no-ref">（本次回答无原文引用）</div>
      </el-card>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { api, type QueryAnswer } from '../api'

const question = ref('')
const loading = ref(false)

interface HistoryItem {
  question: string
  html: string
  refs: QueryAnswer['refs']
}
const history = ref<HistoryItem[]>([])

async function ask() {
  const q = question.value.trim()
  if (!q) {
    ElMessage.warning('请输入问题')
    return
  }
  loading.value = true
  try {
    const res = await api.nlQuery(q)
    const data = res.data
    history.value.unshift({
      question: q,
      html: DOMPurify.sanitize(marked.parse(data?.answer || '', { async: false }) as string),
      refs: data?.refs || [],
    })
    question.value = ''
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.toolbar { margin-bottom: 14px; }
.card-title { font-size: 16px; font-weight: 600; margin-right: 12px; }
.card-sub { font-size: 12px; color: #909399; }
.ask-bar { display: flex; gap: 10px; margin-bottom: 16px; }
.history { display: flex; flex-direction: column; gap: 12px; }
.qa-card { background: var(--ch-surface-2); }
.q { font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 8px; display: flex; align-items: center; gap: 6px; }
.a { color: var(--el-text-color-primary); }
.refs { margin-top: 10px; border-top: 1px dashed var(--ch-line-soft); padding-top: 8px; }
.refs-title { font-size: 12px; color: var(--el-text-color-secondary); margin-bottom: 6px; }
.ref-item { display: flex; align-items: flex-start; gap: 8px; margin-bottom: 6px; }
.ref-item .el-tag { flex-shrink: 0; }
.ref-quote {
  margin: 0; padding: 4px 8px; border-left: 3px solid var(--el-color-primary);
  background: var(--ch-surface-3); color: var(--el-text-color-regular); font-size: 12px; white-space: pre-wrap; word-break: break-word;
}
.no-ref { margin-top: 8px; font-size: 12px; color: var(--el-text-color-placeholder); }
/* markdown 基础排版（同条款审查页） */
.md-body { line-height: 1.9; word-break: break-word; }
.md-body ul, .md-body ol { padding-left: 22px; margin: 6px 0; }
.md-body strong { color: #c0392b; }
.md-body h1, .md-body h2, .md-body h3 { margin: 10px 0 6px; font-weight: 600; }
</style>
