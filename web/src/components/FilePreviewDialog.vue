<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="860px"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div v-loading="loading" class="pv-wrap">
      <el-empty v-if="!loading && error" :description="error" :image-size="80" />
      <iframe v-else-if="html" :srcdoc="html" class="pv-frame" frameborder="0"></iframe>
    </div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
      <el-button type="primary" :disabled="!fileId" @click="download">下载原件</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api } from '../api'

// 在线预览弹框：docx/txt/pdf 正文由服务端转成自包含 HTML，用 iframe srcdoc 呈现。
// 浏览器原生打不开 .docx（只会触发下载），所以统一走 /files/{id}/preview-html。
const props = defineProps<{
  modelValue: boolean
  fileId: number | null
  filename?: string
}>()
const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})
const title = computed(() => (props.filename ? `在线预览 · ${props.filename}` : '在线预览'))

const loading = ref(false)
const error = ref('')
const html = ref('')

async function load() {
  if (!props.fileId) return
  loading.value = true
  error.value = ''
  html.value = ''
  try {
    const res = await api.filePreviewHtml(props.fileId)
    if (res.code === 0 && res.data?.html) html.value = res.data.html
    else error.value = res.message || '预览加载失败'
  } catch (e: any) {
    error.value = e?.message || '预览加载失败'
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (v) => {
    if (v) load()
  },
)

function download() {
  if (props.fileId) window.open(api.fileDownloadUrl(props.fileId), '_blank')
}
</script>

<style scoped>
.pv-wrap {
  min-height: 420px;
}
.pv-frame {
  width: 100%;
  height: 62vh;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  background: #f5f6f8;
}
</style>