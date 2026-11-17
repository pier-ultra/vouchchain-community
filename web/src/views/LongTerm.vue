<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">长期保存（国密合规）</span>
        <span class="card-sub">A5：SM3 摘要 + SM4 加密落盘 + 时间戳登记 + 版本控制（2028-01-01 财会 9 号硬窗口）</span>
      </div>
      <div>
        <el-input v-model="query.sourceType" placeholder="源类型 invoice/contract" clearable style="width: 180px" @keyup.enter="load" @clear="load" />
        <el-button type="primary" plain @click="load">查询</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" style="margin-bottom: 12px"
      title="合规封装层：不重造密码算法，使用国密库（tjfoc/gmsm）。生产需配置 LONGTERM_SM4_KEY（16 字节 hex）；未配置时用内置开发密钥，仅限本地演示。" />

    <el-form inline class="arch-form">
      <el-form-item label="源类型">
        <el-select v-model="form.sourceType" style="width: 150px">
          <el-option label="invoice" value="invoice" />
          <el-option label="contract" value="contract" />
          <el-option label="voucher" value="voucher" />
        </el-select>
      </el-form-item>
      <el-form-item label="源ID">
        <el-input-number v-model="form.sourceId" :min="1" style="width: 120px" />
      </el-form-item>
      <el-form-item label="源件路径">
        <el-input v-model="form.srcPath" placeholder="如 uploads/xxx.pdf 或 C:\...\invoice.ofd" style="width: 260px" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="archiving" @click="archive">归档登记</el-button>
      </el-form-item>
    </el-form>

    <el-table :data="rows" v-loading="loading" border stripe size="small">
      <el-table-column prop="id" label="ID" width="56" />
      <el-table-column prop="sourceType" label="源类型" width="90" />
      <el-table-column prop="sourceId" label="源ID" width="70" />
      <el-table-column prop="version" label="版本" width="60" />
      <el-table-column prop="digestAlg" label="摘要" width="70">
        <template #default="{ row }"><el-tag size="small" type="info">{{ row.digestAlg }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="digest" label="SM3 摘要" min-width="200" show-overflow-tooltip />
      <el-table-column prop="encryptAlg" label="加密" width="70">
        <template #default="{ row }"><el-tag size="small" type="success">{{ row.encryptAlg }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="tsTime" label="时间戳" width="150" />
      <el-table-column label="校验" width="200" fixed="right">
        <template #default="{ row }">
          <el-button link size="small" type="primary" @click="verify(row)">防篡改校验</el-button>
          <span v-if="row._verify" :class="row._verify.digestOk === false || row._verify.decryptOk === false ? 'v-fail' : 'v-ok'">
            {{ row._verify.digestOk === false || row._verify.decryptOk === false ? '异常' : '正常' }}
          </span>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, type LongTermRow } from '../api'

const rows = ref<LongTermRow[]>([])
const loading = ref(false)
const archiving = ref(false)
const query = reactive({ sourceType: '' })
const form = reactive({ sourceType: 'invoice', sourceId: 1, srcPath: '', remark: '' })

async function load() {
  loading.value = true
  try {
    const res = await api.longTermList({ sourceType: query.sourceType || undefined })
    rows.value = res.data || []
  } finally {
    loading.value = false
  }
}

async function archive() {
  if (!form.srcPath) {
    ElMessage.warning('请填源件路径')
    return
  }
  archiving.value = true
  try {
    const res = await api.longTermArchive({ ...form })
    const d = res.data
    ElMessage.success(`归档登记成功：v${d?.version}，SM3=${d?.digest?.slice(0, 16)}…`)
    await load()
  } finally {
    archiving.value = false
  }
}

async function verify(row: LongTermRow) {
  const res = await api.longTermVerify(row.id)
  row._verify = res.data as any
  const d = res.data
  const ok = d?.digestOk !== false && d?.decryptOk !== false
  ElMessage[ok ? 'success' : 'error'](ok ? '校验正常（摘要+解密一致）' : '校验异常：可能被篡改')
}

onMounted(load)
</script>

<style scoped>
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.arch-form {
  margin-bottom: 12px;
}
.v-ok {
  color: #67c23a;
  font-size: 12px;
}
.v-fail {
  color: #f56c6c;
  font-size: 12px;
}
</style>
