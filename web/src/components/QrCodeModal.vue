<template>
  <el-dialog
    v-model="visible"
    :title="title"
    :width="dialogWidth"
    :fullscreen="false"
    :close-on-click-modal="true"
    :destroy-on-close="true"
    :show-close="true"
    modal-class="qr-overlay-blur"
    class="qr-modal"
  >
    <div class="qr-modal-content" v-if="visible && currentQr">
      <img :src="currentQr.src" :alt="currentQr.alt" class="qr-image" />
      <p class="qr-caption">{{ currentQr.caption }}</p>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'

interface QrInfo {
  src: string
  alt: string
  caption: string
}

const props = defineProps({
  title: { type: String, default: '' },
  qrList: { type: Array, default: () => [] },
})

const currentQr = ref<QrInfo | null>(null)
const visible = ref(false)
const dialogWidth = '480px'

function openQr(qr: QrInfo) {
  currentQr.value = qr
  visible.value = true
}

// 暴露给父组件调用
defineExpose({ openQr })
</script>

<style scoped>
.qr-modal :deep(.el-dialog__header) {
  padding-bottom: 8px;
  border-bottom: 1px solid var(--el-border-color-light);
}
.qr-modal :deep(.el-dialog__body) {
  padding: 0;
  overflow: hidden;
}
.qr-modal-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 24px 32px 28px;
}
.qr-image {
  width: 420px;
  max-width: 72vw;
  height: auto;
  aspect-ratio: 1 / 1;
  border-radius: 12px;
  border: 1px solid var(--el-border-color-light);
  background: #fff;
  padding: 12px;
  box-sizing: border-box;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
  object-fit: contain;
}
.qr-caption {
  margin-top: 16px;
  font-size: 15px;
  font-weight: 500;
  color: var(--el-text-color-primary);
  text-align: center;
}
</style>

<style>
/* 弹窗遮罩：模糊背景 + 加深，前景对话框清晰（二维码清晰可扫） */
.qr-overlay-blur {
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
  background: rgba(15, 15, 25, 0.55) !important;
}
</style>