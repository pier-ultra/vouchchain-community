<template>
  <el-card>
    <div class="toolbar">
      <div>
        <span class="card-title">AI 生成合同</span>
        <span class="card-sub">简略信息 / 一段话 / 参考文件 → AI 起草合同全文 → 自动进流水线（抽取→复核→审批→签署）</span>
      </div>
    </div>

    <el-row :gutter="20">
      <!-- 左：要素表单 -->
      <el-col :span="14">
        <el-form label-width="100px">
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
            <el-button type="primary" size="large" :loading="generating" :disabled="generating" @click="generate">
              <el-icon><MagicStick /></el-icon>
              AI 生成合同
            </el-button>
            <span v-if="generating" class="gen-hint">生成中，过程见右侧日志…</span>
          </el-form-item>
        </el-form>
      </el-col>

      <!-- 右：生成过程日志 + 结果 -->
      <el-col :span="10">
        <el-card shadow="never" class="log-card">
          <template #header>
            <div class="log-head">
              <span>生成过程</span>
              <el-tag v-if="generating" size="small" type="warning">进行中</el-tag>
              <el-tag v-else-if="result" size="small" type="success">已完成</el-tag>
              <el-tag v-else-if="failMsg" size="small" type="danger">失败</el-tag>
              <span v-else class="log-idle">尚未开始</span>
            </div>
          </template>
          <div ref="logBox" class="log-box">
            <div v-if="!logs.length" class="log-empty">点击「AI 生成合同」后，这里会实时显示 AI 每一步在做什么。</div>
            <div v-for="(l, i) in logs" :key="i" class="log-line" :class="'lv-' + (l.level || 'info')">
              <span class="log-time">{{ l.time }}</span>
              <span class="log-step">{{ l.step }}</span>
              <span class="log-msg">{{ l.msg }}</span>
            </div>
            <div v-if="failMsg" class="log-line lv-error">
              <span class="log-msg">{{ failMsg }}</span>
            </div>
          </div>
        </el-card>

        <el-card v-if="result" shadow="never" class="res-card">
          <template #header>
            <div class="log-head">
              <span>生成结果</span>
              <el-tag size="small" type="info">Word 文档</el-tag>
            </div>
          </template>
          <el-descriptions :column="1" size="small" border>
            <el-descriptions-item label="文件名">{{ result.filename }}</el-descriptions-item>
            <el-descriptions-item label="规格">
              {{ fmtSize(result.size) }} · {{ result.chars }} 字 · {{ result.lines }} 段 · {{ result.clauses }} 条条款
            </el-descriptions-item>
            <el-descriptions-item label="合同任务">#{{ result.fileId }}（已入流水线，状态 {{ result.status }}）</el-descriptions-item>
            <el-descriptions-item label="文件中心">#{{ result.docxFileId }}（业务类型 contract）</el-descriptions-item>
          </el-descriptions>
          <div class="res-actions">
            <el-button type="primary" size="small" @click="openPreview">在线预览</el-button>
            <el-button size="small" @click="downloadDocx">下载 docx</el-button>
            <el-button size="small" @click="goTaskCenter">查看流水线进度</el-button>
            <el-button size="small" @click="goFileCenter">去文件中心</el-button>
          </div>
          <el-alert
            class="res-tip"
            type="info"
            :closable="false"
            title="生成内容以草稿定位，正式使用前须经人工复核与审批"
          />
        </el-card>
      </el-col>
    </el-row>

    <FilePreviewDialog v-model="previewOpen" :file-id="result?.docxFileId || null" :filename="result?.filename" />
  </el-card>
</template>

<script setup lang="ts">
import { nextTick, reactive, ref, watch } from 'vue'
import type { UploadFile } from 'element-plus'
import { api } from '../api'
import type { GenLogEvent, GenResultData } from '../api'
import FilePreviewDialog from '../components/FilePreviewDialog.vue'

const types = ['采购', '销售', '租赁', '服务', '其他']
const generating = ref(false)
const refFile = ref<File | null>(null)
const logs = ref<GenLogEvent[]>([])
const result = ref<GenResultData | null>(null)
const failMsg = ref('')
const previewOpen = ref(false)
const logBox = ref<HTMLElement | null>(null)

const form = reactive({
  contractType: '采购',
  partyA: '',
  partyB: '',
  subject: '',
  amount: '',
  term: '',
  notes: '',
})

function fmtSize(n: number): string {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(2) + ' MB'
}

function onRefPick(uploadFile: UploadFile) {
  refFile.value = uploadFile.raw || null
}

// 日志实时追加后滚到底部，保证「过程可见」
watch(
  () => logs.value.length,
  async () => {
    await nextTick()
    if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight
  },
)

async function generate() {
  generating.value = true
  logs.value = []
  result.value = null
  failMsg.value = ''
  try {
    await api.generateContractStream(
      { ...form },
      refFile.value,
      {
        onLog: (e) => {
          logs.value.push(e)
        },
        onDone: (d) => {
          if (d.code === 0 && d.data) {
            result.value = d.data
            ElMessage.success('合同已生成：Word 文档已存入文件中心并进入流水线')
          } else {
            failMsg.value = d.message || '生成失败'
            ElMessage.error(failMsg.value)
          }
        },
        onError: (msg) => {
          failMsg.value = msg
          ElMessage.error(msg)
        },
      },
    )
  } finally {
    generating.value = false
  }
}

function openPreview() {
  if (result.value?.docxFileId) previewOpen.value = true
}
function downloadDocx() {
  if (result.value?.docxFileId) window.open(api.fileDownloadUrl(result.value.docxFileId), '_blank')
}
function goTaskCenter() {
  location.hash = '#/extraction'
}
function goFileCenter() {
  location.hash = '#/files'
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
.gen-hint {
  margin-left: 10px;
  font-size: 12px;
  color: #e6a23c;
}
.log-card {
  margin-bottom: 14px;
}
.log-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
}
.log-idle {
  font-size: 12px;
  font-weight: 400;
  color: #c0c4cc;
}
.log-box {
  height: 430px;
  overflow-y: auto;
  padding: 8px 10px;
  background: #fafbfc;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  font-family: 'Consolas', 'Courier New', monospace;
}
.log-empty {
  font-size: 12px;
  color: #909399;
  padding: 8px 4px;
}
.log-line {
  display: flex;
  gap: 8px;
  padding: 4px 0;
  font-size: 12px;
  line-height: 1.6;
  border-bottom: 1px dashed #f0f2f5;
}
.log-line:last-child {
  border-bottom: none;
}
.log-time {
  flex: none;
  color: #909399;
}
.log-step {
  flex: none;
  color: #409eff;
  font-weight: 600;
}
.log-msg {
  color: #303133;
  word-break: break-all;
}
.lv-warn .log-msg {
  color: #e6a23c;
}
.lv-error .log-msg,
.lv-error .log-time,
.lv-error .log-step {
  color: #f56c6c;
}
.res-card {
  margin-bottom: 14px;
}
.res-actions {
  margin-top: 12px;
}
.res-tip {
  margin-top: 12px;
}
</style>