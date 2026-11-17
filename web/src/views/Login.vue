<template>
  <div class="login-page">
    <!-- 左：品牌叙事区（窄屏隐藏） -->
    <section class="login-showcase">
      <div class="showcase-grid"></div>
      <div class="showcase-glow"></div>
      <div class="showcase-inner">
        <div class="showcase-brand">
          <span class="showcase-mark">C</span>
          <span>VouchChain</span>
        </div>
        <div class="showcase-copy">
          <div class="showcase-eyebrow">CONTRACT LIFECYCLE PLATFORM</div>
          <h1>让每一份合同<br />都有迹可循</h1>
          <p>从台账管理、AI 生成到抽取复核与审批签署，一站式管理企业合同全生命周期。</p>
          <div class="showcase-features">
            <div class="showcase-feature"><el-icon><Document /></el-icon><span>合同台账与相对方画像</span></div>
            <div class="showcase-feature"><el-icon><MagicStick /></el-icon><span>AI 起草与智能抽取</span></div>
            <div class="showcase-feature"><el-icon><Stamp /></el-icon><span>审批签署全程留痕</span></div>
          </div>
        </div>
        <div class="showcase-community">
          <div class="community-qr">
            <img src="/community/qrcode-qqgroup.jpg" alt="QQ 群：VouchChain 开源社区" title="QQ 群：VouchChain 合同与履约中枢 · 开源社区" />
            <span>QQ 群 · 开源社区</span>
          </div>
          <div class="community-qr">
            <img src="/community/qrcode-wechat-personal.jpg" alt="作者微信" title="微信：添加作者，拉你进 VouchChain 社区版交流群" />
            <span>微信 · 拉你进群</span>
          </div>
        </div>
        <div class="showcase-foot">© 2026 VouchChain · 企业内部合规平台</div>
      </div>
    </section>

    <!-- 右：登录表单 -->
    <section class="login-panel">
      <div class="login-form-wrap">
        <div class="form-eyebrow">WELCOME BACK</div>
        <h2 class="login-title">登录工作台</h2>
        <p class="login-sub">欢迎回来，请登录以继续</p>

        <!-- dev 演示态：任意账号 + 角色模拟 -->
        <el-form v-if="cfg.devMode" label-position="top" @submit.prevent="onSubmit">
          <el-form-item label="用户名">
            <el-input v-model="form.user" placeholder="dev" clearable />
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="form.pass" type="password" placeholder="dev" show-password />
          </el-form-item>
          <el-form-item label="角色（dev 模式模拟 RBAC）">
            <el-select v-model="form.role" style="width: 100%">
              <el-option v-for="(label, key) in roleLabels" :key="key" :label="label" :value="key" />
            </el-select>
          </el-form-item>
          <el-button type="primary" style="width: 100%" @click="onSubmit">登 录</el-button>
        </el-form>

        <!-- SSO 态：Casdoor OAuth 跳转 / 直接粘贴应用签发的 Access Token（离线自托管也可用） -->
        <div v-else class="sso-box">
          <el-button type="primary" style="width: 100%" :disabled="!cfg.authorizeBase" @click="gotoCasdoor">
            使用 Casdoor 单点登录
          </el-button>
          <div class="sso-hint">
            {{ cfg.authorizeBase ? '跳转到 Casdoor 登录页，回调后自动换取 token' : '未配置 Casdoor 实例地址，可直接粘贴 Access Token' }}
          </div>
          <el-divider>或直接粘贴 Access Token</el-divider>
          <el-input v-model="tokenInput" type="textarea" :rows="3" placeholder="粘贴 Casdoor 应用签发的 JWT" />
          <el-button style="width: 100%; margin-top: 10px" :loading="verifying" @click="pasteLogin">校验并登录</el-button>
          <div class="sso-hint">
            验签纯本地进行，无需运行 Casdoor 服务；角色取自 token 内的 roles。
          </div>
        </div>

        <el-alert
          style="margin-top: 18px"
          :type="cfg.devMode ? 'info' : cfg.certConfigured ? 'success' : 'warning'"
          :closable="false"
          :title="cfg.devMode ? 'dev 演示态：任意账号密码可登录，角色模拟 RBAC' : cfg.certConfigured ? 'SSO 态：应用证书已配置，Bearer token 验签生效' : 'SSO 态：应用证书未配置，请求将被拒绝'"
          :description="cfg.devMode
            ? '设置 APP_DEV_MODE=false + CASDOOR_CERTIFICATE 即切换为生产验签态'
            : '回调地址 ' + cfg.redirectUri + '（需在 Casdoor 应用里登记）'"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { roleLabels, setSession, type Role } from '../rbac'
import { api, type AuthConfigRow } from '../api'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const form = reactive({ user: 'dev', pass: '', role: 'admin' as Role })
const cfg = ref<AuthConfigRow>({
  devMode: true, ssoEnabled: false, endpoint: '', clientId: '', orgName: '', appName: '',
  certConfigured: false, redirectUri: '', authorizeBase: '',
})
const tokenInput = ref('')
const verifying = ref(false)

async function loadConfig() {
  try {
    const res = await api.authConfig()
    if (res.data) cfg.value = res.data
  } catch { /* 服务不可达时按 dev 表单展示 */ }
}

function onSubmit() {
  if (!form.user.trim()) {
    ElMessage.warning('请输入用户名')
    return
  }
  auth.login(form.user.trim())
  localStorage.setItem('ch_role', form.role)
  ElMessage.success(`登录成功（dev 模式 · ${roleLabels[form.role]}）`)
  router.push('/')
}

function gotoCasdoor() {
  const state = Math.random().toString(36).slice(2) + Date.now().toString(36)
  sessionStorage.setItem('ch_oauth_state', state)
  window.location.href = cfg.value.authorizeBase + encodeURIComponent(state)
}

// 粘贴 token：先落到本地，再用 /api/me 真实验签，失败即清除（后端会 401）
async function pasteLogin() {
  const token = tokenInput.value.trim()
  if (!token) {
    ElMessage.warning('请粘贴 Access Token')
    return
  }
  verifying.value = true
  const prev = localStorage.getItem('ch_token') || ''
  localStorage.setItem('ch_token', token)
  try {
    const res = await api.me()
    if (res.code !== 0 || !res.data) throw new Error(res.message || '验签失败')
    auth.login(res.data.username || res.data.displayName, token)
    setSession(res.data)
    ElMessage.success(`登录成功：${res.data.displayName} · ${res.data.roleName}`)
    router.push('/')
  } catch {
    localStorage.setItem('ch_token', prev)
    // 拦截器已弹出后端的具体拒绝原因
  } finally {
    verifying.value = false
  }
}

// OAuth 回调：/#/login?code=...&state=...
async function handleCallback() {
  const code = String(route.query.code || '')
  const state = String(route.query.state || '')
  if (!code) return
  if (state && state !== sessionStorage.getItem('ch_oauth_state')) {
    ElMessage.error('state 校验失败，请重新登录')
    return
  }
  sessionStorage.removeItem('ch_oauth_state')
  verifying.value = true
  try {
    const res = await api.authCallback(code, state)
    if (res.code !== 0 || !res.data) throw new Error(res.message || '换取 token 失败')
    localStorage.setItem('ch_token', res.data.token)
    const me = await api.me()
    auth.login(res.data.username, res.data.token)
    if (me.data) setSession(me.data)
    ElMessage.success('单点登录成功')
    router.replace('/')
  } catch {
    localStorage.removeItem('ch_token')
  } finally {
    verifying.value = false
  }
}

onMounted(async () => {
  await loadConfig()
  await handleCallback()
})
</script>

<style scoped>
.login-page {
  height: 100vh;
  display: flex;
  background: var(--ch-canvas);
}

/* ---- 左：品牌叙事 ---- */
.login-showcase {
  position: relative;
  flex: 1 1 58%;
  min-width: 0;
  overflow: hidden;
  background: var(--ch-ink);
  color: #fff;
}
.showcase-grid {
  position: absolute; inset: 0;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.035) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.035) 1px, transparent 1px);
  background-size: 44px 44px;
  -webkit-mask-image: radial-gradient(ellipse at 30% 40%, #000 10%, transparent 75%);
  mask-image: radial-gradient(ellipse at 30% 40%, #000 10%, transparent 75%);
  pointer-events: none;
}
.showcase-glow {
  position: absolute; top: -180px; right: -160px;
  width: 620px; height: 620px;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.28), rgba(79, 70, 229, 0.06) 45%, transparent 70%);
  pointer-events: none;
}
.showcase-inner {
  position: relative; z-index: 1;
  height: 100%;
  display: flex; flex-direction: column;
  padding: 42px 56px 36px;
}
.showcase-brand {
  display: flex; align-items: center; gap: 12px;
  font-size: 15px; font-weight: 650; letter-spacing: -0.01em;
  color: #f8fafc;
}
.showcase-mark {
  display: inline-flex; align-items: center; justify-content: center;
  width: 32px; height: 32px; border-radius: 9px;
  background: linear-gradient(145deg, #6366f1, #4338ca);
  border: 1px solid rgba(255, 255, 255, 0.18);
  font-size: 15px; font-weight: 800; letter-spacing: -0.04em;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.2);
}
.showcase-copy { margin: auto 0; max-width: 520px; }
.showcase-eyebrow {
  font-size: 10px; font-weight: 700; letter-spacing: 0.2em;
  color: #a5b4fc;
}
.showcase-copy h1 {
  margin: 18px 0;
  font-size: clamp(34px, 4vw, 48px);
  font-weight: 700; line-height: 1.28; letter-spacing: -0.045em;
  color: #fff;
}
.showcase-copy > p {
  margin: 0; max-width: 420px;
  font-size: 14px; line-height: 1.9; color: #aab4cc;
}
.showcase-features {
  margin-top: 38px;
  display: flex; flex-direction: column; gap: 14px;
}
.showcase-feature {
  display: flex; align-items: center; gap: 12px;
  font-size: 13px; color: #dbe2f0;
}
.showcase-feature .el-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: 30px; height: 30px; border-radius: 8px;
  background: rgba(129, 140, 248, 0.14); color: #a5b4fc; font-size: 15px;
}
.showcase-community {
  display: flex; gap: 16px; margin-top: 22px; padding-top: 18px;
  border-top: 1px solid rgba(129, 140, 248, 0.12);
}
.community-qr {
  display: flex; flex-direction: column; align-items: center; gap: 6px;
}
.community-qr img {
  width: 86px; height: 86px; border-radius: 10px;
  border: 1px solid rgba(129, 140, 248, 0.25);
  background: #fff; padding: 4px; box-sizing: border-box;
}
.community-qr span {
  font-size: 11px; color: #6b7691;
}
.showcase-foot {
  font-size: 11px; color: #6b7691; letter-spacing: 0.03em;
}

/* ---- 右：表单 ---- */
.login-panel {
  flex: 0 0 480px;
  display: flex; align-items: center; justify-content: center;
  padding: 40px;
  background: var(--ch-surface);
  border-left: 1px solid var(--ch-line-soft);
}
.login-form-wrap { width: 100%; max-width: 360px; }
.form-eyebrow {
  font-size: 10px; font-weight: 700; letter-spacing: 0.2em;
  color: var(--ch-accent); opacity: 0.8;
}
.login-title {
  margin: 10px 0 6px;
  font-size: 25px; font-weight: 700; letter-spacing: -0.035em;
  color: var(--el-text-color-primary);
}
.login-sub {
  margin: 0 0 28px;
  font-size: 13px; color: var(--el-text-color-secondary);
}
.login-panel :deep(.el-form-item__label) {
  font-size: 12px; font-weight: 600; color: var(--el-text-color-regular);
}
.login-panel :deep(.el-button--primary) {
  height: 42px; border-radius: 8px; font-weight: 600; font-size: 14px;
  margin-top: 4px;
}
.sso-hint {
  margin-top: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px; line-height: 1.6;
}

@media (max-width: 980px) {
  .login-showcase { display: none; }
  .login-panel { flex: 1; padding: 32px 24px; }
}
</style>
