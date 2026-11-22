<template>
  <div class="about-page">
    <el-breadcrumb separator="/">
      <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
      <el-breadcrumb-item>关于我们</el-breadcrumb-item>
    </el-breadcrumb>

    <div class="about-content">
      <!-- 头部区域 -->
      <div class="about-header">
        <div class="header-brand">
          <span class="brand-mark">C</span>
          <span>VouchChain</span>
        </div>
        <h1>企业内部合规与合同履约中枢</h1>
        <p class="subtitle">从合同台账、AI 生成到抽取复核、审批签署、发票三方匹配、费控收入确认、用印管理——一站式管理企业合同全生命周期与财务合规闭环。</p>
      </div>

      <el-divider />

      <!-- 技术栈 -->
      <section class="about-section">
        <h2><el-icon><SetUp /></el-icon>技术栈</h2>
        <div class="tech-grid">
          <el-card class="tech-card" v-for="t in techStack" :key="t.title">
            <template #header>
              <div class="card-header">
                <el-icon :class="t.iconClass"><component :is="t.icon" /></el-icon>
                <span>{{ t.title }}</span>
              </div>
            </template>
            <div class="tech-tags">
              <el-tag v-for="tag in t.tags" :key="tag" size="small" effect="plain">{{ tag }}</el-tag>
            </div>
          </el-card>
        </div>
      </section>

      <el-divider />

      <!-- 支持的数据库 -->
      <section class="about-section">
        <h2><el-icon><Server /></el-icon>支持的数据库</h2>
        <div class="db-grid">
          <el-tag v-for="db in databases" :key="db" size="large" effect="plain" :class="db.class">
            <el-icon><component :is="db.icon" /></el-icon>
            {{ db.name }}
          </el-tag>
        </div>
      </section>

      <el-divider />

      <!-- 核心功能 -->
      <section class="about-section">
        <h2><el-icon><Grid /></el-icon>核心功能模块</h2>
        <div class="feature-grid">
          <el-card class="feature-card" v-for="f in features" :key="f.title">
            <template #header>
              <div class="card-header">
                <el-icon :class="f.iconClass"><component :is="f.icon" /></el-icon>
                <span>{{ f.title }}</span>
              </div>
            </template>
            <el-tag v-for="tag in f.tags" :key="tag" size="small" effect="plain">{{ tag }}</el-tag>
          </el-card>
        </div>
      </section>

      <el-divider />

      <!-- 社区与联系 -->
      <section class="about-section">
        <h2><el-icon><ChatDotRound /></el-icon>社区与联系</h2>
        <div class="community-grid">
          <el-card class="community-card" shadow="never">
            <div class="community-item">
              <div class="qr-wrapper" @click="qrModalRef?.openQr(communityQrs[0])">
                <img :src="communityQrs[0].src" :alt="communityQrs[0].alt" class="community-qr-img" />
                <div class="qr-overlay">
                  <el-icon><ZoomIn /></el-icon>
                  <span>点击查看大图</span>
                </div>
              </div>
              <div class="community-info">
                <h4>{{ communityQrs[0].caption }}</h4>
                <p>{{ communityQrs[0].desc }}</p>
              </div>
            </div>
          </el-card>
          <el-card class="community-card" shadow="never">
            <div class="community-item">
              <div class="qr-wrapper" @click="qrModalRef?.openQr(communityQrs[1])">
                <img :src="communityQrs[1].src" :alt="communityQrs[1].alt" class="community-qr-img" />
                <div class="qr-overlay">
                  <el-icon><ZoomIn /></el-icon>
                  <span>点击查看大图</span>
                </div>
              </div>
              <div class="community-info">
                <h4>{{ communityQrs[1].caption }}</h4>
                <p>{{ communityQrs[1].desc }}</p>
              </div>
            </div>
          </el-card>
        </div>
      </section>

      <el-divider />

      <!-- 底部版权 -->
      <div class="about-footer">
        <p>© 2026 VouchChain · 企业内部合规平台</p>
        <p class="version">版本 v0.3.4 | MIT License | Powered by GoFrame + Vue 3 + Element Plus</p>
      </div>
    </div>

    <!-- 二维码放大模态框 -->
    <QrCodeModal ref="qrModalRef" title="扫码加入" :qr-list="communityQrs" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import QrCodeModal from '../components/QrCodeModal.vue'

// 技术栈数据
const techStack = [
  {
    title: '后端框架',
    icon: 'SetUp',
    iconClass: 'tech-icon-backend',
    tags: ['GoFrame v2', 'Go 1.23+', '纯 Go 无 CGO', '模块化设计']
  },
  {
    title: '前端框架',
    icon: 'Monitor',
    iconClass: 'tech-icon-frontend',
    tags: ['Vue 3', 'TypeScript', 'Element Plus', 'Vite', 'Pinia']
  },
  {
    title: '数据库 & ORM',
    icon: 'Server',
    iconClass: 'tech-icon-db',
    tags: ['SQLite', 'PostgreSQL', 'MySQL 8/TiDB', 'openGauss', 'DM8', 'KingBase', 'OceanBase', 'GORM-like']
  },
  {
    title: 'AI & OCR',
    icon: 'MagicStick',
    iconClass: 'tech-icon-ai',
    tags: ['OpenAI 兼容 API', 'RapidOCR', 'Sherpa-ONNX', '流式输出', 'Function Calling']
  },
  {
    title: '认证 & 安全',
    icon: 'Lock',
    iconClass: 'tech-icon-auth',
    tags: ['Casdoor OIDC', 'JWT RS256', 'RBAC', '数据强隔离', '国密 SM2/SM4']
  },
  {
    title: '部署 & 运维',
    icon: 'Box',
    iconClass: 'tech-icon-deploy',
    tags: ['单二进制分发', 'go:embed 内嵌', '自动迁移', '零配置启动', 'Docker 支持']
  }
]

// 数据库支持
const databases = [
  { name: 'SQLite', icon: 'Database', class: 'db-sqlite' },
  { name: 'PostgreSQL', icon: 'Database', class: 'db-pg' },
  { name: 'MySQL 8 / TiDB', icon: 'Database', class: 'db-mysql' },
  { name: 'openGauss', icon: 'Database', class: 'db-og' },
  { name: 'DM8 (达梦)', icon: 'Database', class: 'db-dm' },
  { name: 'KingBase (人大金仓)', icon: 'Database', class: 'db-kb' },
  { name: 'OceanBase', icon: 'Database', class: 'db-ob' }
]

// 核心功能
const features = [
  {
    title: '合同全生命周期',
    icon: 'Files',
    iconClass: 'feat-icon-contract',
    tags: ['合同台账', 'AI 起草/生成', '模板库/变量套打', '义务看板', '审批签署', '相对方画像']
  },
  {
    title: '发票与财务闭环',
    icon: 'Coin',
    iconClass: 'feat-icon-finance',
    tags: ['发票中心/验真/归档', '三方匹配(票-合同-付款)', '四性归档(真/全/合/关)', '费控/报销/差标', '收入确认', '用印管理']
  },
  {
    title: 'AI 智能化',
    icon: 'MagicStick',
    iconClass: 'feat-icon-ai',
    tags: ['合同/发票智能抽取', '条款复核修正', 'AI 分录草稿', '智能问答', '模型配置管理']
  },
  {
    title: '组织与主数据',
    icon: 'OfficeBuilding',
    iconClass: 'feat-icon-org',
    tags: ['多主体/多组织', '主数据雏形', 'MDM 去重', 'K3/KIS 对接']
  },
  {
    title: '治理与合规',
    icon: 'CircleCheck',
    iconClass: 'feat-icon-gov',
    tags: ['合规策略引擎', 'RBAC 权限', '强隔离策略', '审计日志', '等保证据链', 'SCIM 身份同步']
  },
  {
    title: '长期保存与国密',
    icon: 'Lock',
    iconClass: 'feat-icon-long',
    tags: ['国密 SM2/SM4', '时间戳/区块链存证', '归档调度', '电子签适配']
  }
]

// 社区二维码
const communityQrs = ref([
  {
    src: '/community/qrcode-qqgroup.jpg',
    alt: 'QQ 群：VouchChain 开源社区',
    caption: 'QQ 群 · VouchChain 开源社区',
    desc: '扫码加入 QQ 群，技术交流、问题反馈、版本更新第一时间获取'
  },
  {
    src: '/community/qrcode-wechat-personal.jpg',
    alt: '作者微信',
    caption: '微信 · 添加作者',
    desc: '添加作者微信，拉你进 VouchChain 社区版微信交流群'
  }
])

const qrModalRef = ref<InstanceType<typeof QrCodeModal> | null>(null)
</script>

<style scoped>
.about-page {
  padding: 24px;
  max-width: 1100px;
  margin: 0 auto;
}
.about-content {
  margin-top: 16px;
}
.about-header {
  text-align: center;
  padding: 32px 0;
}
.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(145deg, #6366f1, #4338ca);
  color: #fff;
  font-size: 20px;
  font-weight: 800;
  margin-bottom: 16px;
  box-shadow: 0 4px 16px rgba(99, 102, 241, 0.3);
}
.about-header h1 {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 12px;
  color: var(--el-text-color-primary);
}
.subtitle {
  max-width: 720px;
  margin: 0 auto;
  font-size: 15px;
  line-height: 1.8;
  color: var(--el-text-color-secondary);
}
.about-section {
  margin: 32px 0;
}
.about-section h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 18px;
  font-weight: 650;
  margin-bottom: 20px;
  color: var(--el-text-color-primary);
}
.about-section h2 .el-icon {
  font-size: 18px;
  color: var(--ch-accent);
}

/* 技术栈卡片 */
.tech-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.tech-card {
  transition: transform 0.2s, box-shadow 0.2s;
}
.tech-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 24px rgba(0, 0, 0, 0.08);
}
.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 14px;
}
.tech-icon-backend { color: #6366f1; }
.tech-icon-frontend { color: #42b883; }
.tech-icon-db { color: #f59e0b; }
.tech-icon-ai { color: #ec4899; }
.tech-icon-auth { color: #06b6d4; }
.tech-icon-deploy { color: #8b5cf6; }
.tech-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

/* 数据库标签 */
.db-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.db-grid .el-tag {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  padding: 8px 14px;
  transition: transform 0.2s, box-shadow 0.2s;
}
.db-grid .el-tag:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.1);
}
.db-sqlite { border-color: #0074b4; color: #0074b4; }
.db-pg { border-color: #336791; color: #336791; }
.db-mysql { border-color: #f29111; color: #f29111; }
.db-og { border-color: #e74c3c; color: #e74c3c; }
.db-dm { border-color: #9b59b6; color: #9b59b6; }
.db-kb { border-color: #27ae60; color: #27ae60; }
.db-ob { border-color: #f39c12; color: #f39c12; }

/* 核心功能卡片 */
.feature-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}
.feature-card {
  transition: transform 0.2s, box-shadow 0.2s;
}
.feature-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 24px rgba(0, 0, 0, 0.08);
}
.feat-icon-contract { color: #6366f1; }
.feat-icon-finance { color: #f59e0b; }
.feat-icon-ai { color: #ec4899; }
.feat-icon-org { color: #06b6d4; }
.feat-icon-gov { color: #8b5cf6; }
.feat-icon-long { color: #ef4444; }

/* 社区二维码 */
.community-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 24px;
}
@media (max-width: 768px) {
  .community-grid {
    grid-template-columns: 1fr;
  }
}
.community-card {
  border-radius: 12px;
  overflow: hidden;
}
.community-item {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.qr-wrapper {
  position: relative;
  flex-shrink: 0;
  border-radius: 12px;
  overflow: hidden;
}
.community-qr-img {
  width: 140px;
  height: 140px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: #fff;
  padding: 8px;
  box-sizing: border-box;
}
.qr-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  color: #fff;
  font-size: 13px;
  opacity: 0;
  transition: opacity 0.2s;
  border-radius: 12px;
  padding: 8px;
}
.qr-wrapper:hover .qr-overlay {
  opacity: 1;
}
.community-info {
  flex: 1;
  min-width: 0;
}
.community-info h4 {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.community-info p {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}

/* 底部 */
.about-footer {
  margin-top: 48px;
  padding-top: 24px;
  text-align: center;
  border-top: 1px solid var(--el-border-color-light);
}
.about-footer p {
  margin: 4px 0;
  font-size: 13px;
  color: var(--el-text-color-placeholder);
}
.version {
  font-size: 12px !important;
  color: var(--el-text-color-placeholder) !important;
}
</style>