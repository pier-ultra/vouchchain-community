<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">AI 生成合同</span>
        <span class="card-sub">简略信息 / 一段话 / 参考文件 → AI 起草合同全文 → 自动进流水线（抽取→复核→审批→签署）</span>
      </div>
    </div>

    <el-form label-width="100px" style="max-width: 720px">
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="合同类型">
            <el-select v-model="form.contractType" style="width: 100%">
              <el-option v-for="t in types" :key="t" :label="t" :value="t" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="履行期限">
            <el-input v-model="form.term" placeholder="如：2026-12-01 至 2027-05-31" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="甲方（我方）">
            <el-input v-model="form.partyA" placeholder="如：北京华信科技有限公司" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="乙方（对方）">
            <el-input v-model="form.partyB" placeholder="如：上海蓝海机械制造有限公司" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-form-item label="标的">
        <el-input v-model="form.subject" placeholder="如：工业自动化设备一批（含安装调试）" />
      </el-form-item>
      <el-form-item label="金额">
        <el-input v-model="form.amount" placeholder="如：人民币 150 万元（含税）" />
      </el-form-item>
      <el-form-item label="要点/草稿">
        <el-input
          v-model="form.notes"
          type="textarea"
          :rows="5"
          placeholder="一段话即可：比如『预付50%，验收后付尾款；逾期按千分之三付违约金；质保24个月；争议交北京仲裁』。留空则按要素生成通用条款。"
        />
      </el-form-item>
      <el-form-item label="参考文件">
        <el-upload :show-file-list="false" :auto-upload="false" accept=".pdf,.docx,.txt" :on-change="onRefPick">
          <el-button>
            <el-icon><Paperclip /></el-icon>
            {{ refFile ? refFile.name : '选择参考文件（可选，仅参考风格与结构）' }}
          </el-button>
        </el-upload>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" size="large" :loading="generating" @click="generate">
          <el-icon><MagicStick /></el-icon>
          AI 生成合同
        </el-button>
      </el-form-item>
    </el-form>

    <el-alert
      v-if="generatedId"
      type="success"
      :closable="false"
      :title="`合同已生成（任务 #${generatedId}），自动进入流水线`"
      description="去「任务中心」查看抽取进度与条款复核；生成内容以草稿定位，正式使用前须经人工复核与审批。"
    />
  </el-card>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import type { UploadFile } from 'element-plus'
import { api } from '../api'

const types = ['采购', '销售', '租赁', '服务', '其他']
const generating = ref(false)
const generatedId = ref<number | null>(null)
const refFile = ref<File | null>(null)

const form = reactive({
  contractType: '采购',
  partyA: '',
  partyB: '',
  subject: '',
  amount: '',
  term: '',
  notes: '',
})

function onRefPick(uploadFile: UploadFile) {
  refFile.value = uploadFile.raw || null
}

async function generate() {
  generating.value = true
  generatedId.value = null
  try {
    const res = await api.generateContract({ ...form }, refFile.value || undefined)
    generatedId.value = res.data?.id || null
    ElMessage.success('合同已生成并进入流水线')
  } finally {
    generating.value = false
  }
}
</script>

<style scoped>
.toolbar {
  margin-bottom: 14px;
}
.card-title {
  font-size: 16px;
  font-weight: 600;
  margin-right: 12px;
}
.card-sub {
  font-size: 12px;
  color: #909399;
}
</style>
