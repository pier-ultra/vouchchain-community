<script setup lang="ts">
import { ApiReference } from '@scalar/api-reference'
import '@scalar/api-reference/style.css'

// Scalar 交互式接口文档，指向后端自生成的 OpenAPI 3.0 spec（/openapi.json）
// url 放顶层（v1.78 起废弃 spec.url 前缀）
const configuration = {
  url: '/openapi.json',
  darkMode: false,
  metaData: {
    title: 'VouchChain 接口文档',
    description: '合同与履约中枢 · 社区版',
  },
}
</script>

<template>
  <div class="api-docs">
    <ApiReference :configuration="configuration" />
  </div>
</template>

<style scoped>
.api-docs {
  height: calc(100vh - 60px);
  min-height: 480px;
  overflow: hidden;
}
</style>
