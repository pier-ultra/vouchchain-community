<template>
  <el-container style="height: 100vh">
    <el-aside :width="collapsed ? '72px' : '248px'" class="aside">
      <div class="logo">
        <div v-if="!collapsed" class="brand">
          <span class="brand-mark">C</span>
          <span class="logo-text">VouchChain</span>
        </div>
        <span v-else class="brand-mark brand-mark-collapsed">C</span>
        <el-icon class="collapse-btn" :title="collapsed ? '展开侧栏' : '收起侧栏'" @click="toggleCollapse">
          <component :is="collapsed ? 'Expand' : 'Fold'" />
        </el-icon>
      </div>
      <!-- 大条目下面挂小菜单：分组只负责归类，逐项可见性仍走角色菜单权限 -->
      <el-menu
        ref="menuRef"
        router
        :default-active="route.path"
        :default-openeds="defaultOpens"
        :collapse="collapsed"
        :collapse-transition="false"
      >
        <el-sub-menu v-for="g in visibleGroups" :key="g.key" :index="g.key">
          <template #title>
            <el-icon><component :is="g.icon" /></el-icon>
            <span>{{ g.title }}</span>
          </template>
          <el-menu-item v-for="i in g.items" :key="i.path" :index="i.path">
            <el-icon><component :is="i.icon" /></el-icon>
            <template #title>{{ i.title }}</template>
          </el-menu-item>
        </el-sub-menu>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <span class="title">{{ route.meta.title }}</span>
        <div class="user">
          <!-- 亮/深主题切换 -->
          <el-icon class="theme-toggle" :title="isDark ? '切换到亮色' : '切换到深色'" @click="toggleTheme">
            <component :is="isDark ? 'Sunny' : 'Moon'" />
          </el-icon>
          <!-- K3 主体上下文：切换后全部业务列表按该主体过滤 -->
          <span class="entity-label">主体</span>
          <el-select v-model="entityId" size="small" style="width: 170px" @change="switchEntity">
            <el-option label="全部主体" :value="0" />
            <el-option v-for="e in entityList" :key="e.id" :label="e.name" :value="e.id" />
          </el-select>
          <!-- 父主体递归：勾选后列表包含当前主体及其所有下级主体的数据 -->
          <el-tooltip content="勾选后，合同/发票等列表会递归带上所有下级主体的数据" placement="bottom">
            <el-checkbox v-model="includeSubtree" size="small" :disabled="entityId === 0" @change="switchSubtree">
              含下级
            </el-checkbox>
          </el-tooltip>
          <!-- 消息中心：铃铛未读角标 + 下拉通知列表 -->
          <el-popover placement="bottom-end" width="360" trigger="click" @show="loadNotifications">
            <template #reference>
              <el-badge :value="unread" :hidden="unread === 0" :max="99">
                <el-icon style="cursor: pointer; font-size: 18px"><Bell /></el-icon>
              </el-badge>
            </template>
            <div class="notif-head">
              <span>消息中心</span>
              <el-button link type="primary" size="small" @click="markAllRead">全部已读</el-button>
            </div>
            <div class="notif-list">
              <div v-for="n in notifications" :key="n.id" class="notif-item" :class="{ unread: !n.is_read }"
                @click="markRead(n)">
                <div class="notif-title">{{ n.title }}</div>
                <div class="notif-content">{{ n.content }}</div>
                <div class="notif-time">{{ fmtTime(n.created_at) }}</div>
              </div>
              <div v-if="!notifications.length" class="notif-empty">暂无消息</div>
            </div>
          </el-popover>
          <el-icon><User /></el-icon>
          <span>{{ me?.displayName || auth.user || 'dev' }}</span>
          <el-tag size="small" :type="role === 'admin' ? 'danger' : role === 'legal' ? 'warning' : 'info'"
            :title="me?.authMode === 'sso' ? '身份来自 Casdoor JWT 验签' : 'dev 演示态'">
            {{ me?.roleName || roleLabels[role] || role }}
          </el-tag>
          <el-tag v-if="me && !me.devMode" size="small" type="success" effect="plain">SSO</el-tag>
          <el-button link type="primary" @click="logout">退出</el-button>
        </div>
      </el-header>
      <el-main>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { currentRole, roleLabels, visibleMenus, setSession, sessionVersion, type SessionRow } from '../rbac'
import { menuGroups, groupForPath } from '../menu'
import { api, type NotificationRow, type LegalEntityRow } from '../api'
import { useTheme } from '../composables/useTheme'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { isDark, toggle: toggleTheme } = useTheme()

const role = computed(() => currentRole())
const me = ref<SessionRow | null>(null)
// 会话加载完成后重算菜单；未加载时用 rbac.ts 的静态兜底，避免服务不可达时白屏
function menuVisible(path: string) {
  void sessionVersion.value
  return visibleMenus().includes(path)
}
async function loadMe() {
  try {
    const res = await api.me()
    if (res.data) {
      me.value = res.data
      setSession(res.data)
    }
  } catch { /* 401 已由拦截器处理，其余情况保留兜底菜单 */ }
}

// ---------- 侧栏分组（18 项 → 5 个大条目） ----------
const menuRef = ref<{ open?: (index: string) => void } | null>(null)
const collapsed = ref(localStorage.getItem('ch_menu_collapsed') === '1')
function toggleCollapse() {
  collapsed.value = !collapsed.value
  localStorage.setItem('ch_menu_collapsed', collapsed.value ? '1' : '0')
  if (!collapsed.value) {
    const k = groupForPath(route.path)
    if (k) nextTick(() => menuRef.value?.open?.(k))
  }
}
// 角色过滤后整组无可见子项就直接隐藏，不留下空的大条目
const visibleGroups = computed(() => menuGroups
  .map((g) => ({ ...g, items: g.items.filter((i) => menuVisible(i.path)) }))
  .filter((g) => g.items.length > 0))
const defaultOpens = computed(() => {
  const k = groupForPath(route.path)
  return k && !collapsed.value ? [k] : []
})
// default-openeds 只在首次渲染生效，切页时显式展开当前分组
watch(() => route.path, (p) => {
  const k = groupForPath(p)
  if (k && !collapsed.value) nextTick(() => menuRef.value?.open?.(k))
}, { immediate: true })

// ---------- K3 当前法人主体（顶栏全局切换）+ 递归含下级开关 ----------
const entityId = ref<number>(Number(localStorage.getItem('ch_entity') || 0))
const entityList = ref<LegalEntityRow[]>([])
const includeSubtree = ref(false)
async function loadContext() {
  try {
    const res = await api.context()
    entityList.value = res.data?.entities || []
    includeSubtree.value = !!res.data?.includeSubtree
    if (!localStorage.getItem('ch_entity') && res.data) entityId.value = res.data.legalEntityId
  } catch { /* 服务不可达时保留本地值 */ }
}
async function switchEntity(id: number) {
  await api.contextSwitch(id, includeSubtree.value)
  localStorage.setItem('ch_entity', String(id))
  // 刷新让各页面按新主体重新加载（hash 路由会停留当前页）
  window.location.reload()
}
async function switchSubtree(v: boolean) {
  await api.contextSwitch(entityId.value, v)
  // 刷新让所有已打开的列表按新的主体范围重查
  window.location.reload()
}

// ---------- 消息中心（轮询未读数 + 打开拉详情） ----------
const notifications = ref<NotificationRow[]>([])
const unread = ref(0)
let notifTimer: ReturnType<typeof setInterval> | null = null

async function loadNotifications() {
  try {
    const res = await api.notifications()
    notifications.value = res.data?.list || []
    unread.value = res.data?.unread || 0
  } catch {
    /* 服务不可达时静默 */
  }
}
async function markRead(n: NotificationRow) {
  if (!n.is_read) {
    await api.notificationRead(n.id)
    await loadNotifications()
  }
}
async function markAllRead() {
  await api.notificationReadAll()
  await loadNotifications()
}
function fmtTime(t?: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

onMounted(() => {
  loadMe()
  loadContext()
  loadNotifications()
  notifTimer = setInterval(loadNotifications, 15000)
})
onBeforeUnmount(() => {
  if (notifTimer) clearInterval(notifTimer)
})

function logout() {
  auth.logout()
  setSession(null)
  router.push('/login')
}
</script>

<style scoped>
.aside {
  position: relative;
  background: var(--ch-sidebar-bg);
  transition: width 0.22s cubic-bezier(0.4, 0, 0.2, 1);
  overflow-x: hidden;
  border-right: 1px solid var(--ch-sidebar-border);
}
/* 深色下顶部一道极细高光；亮色下隐藏 */
.aside::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 1px;
  background: var(--ch-sidebar-topline);
  pointer-events: none;
}
.aside :deep(.el-menu) {
  border-right: none;
  background: transparent;
  --el-menu-bg-color: transparent;
  --el-menu-hover-bg-color: transparent;
  --el-menu-text-color: var(--ch-sidebar-text);
  --el-menu-active-color: var(--ch-sidebar-text-active);
}
.aside :deep(.el-menu-item),
.aside :deep(.el-sub-menu__title) {
  height: 44px;
  line-height: 44px;
  margin: 2px 8px;
  border-radius: 8px;
  transition: background-color 0.16s ease, color 0.16s ease;
}
.aside :deep(.el-menu-item:hover),
.aside :deep(.el-sub-menu__title:hover) {
  background-color: var(--ch-sidebar-hover-bg) !important;
  color: var(--ch-sidebar-text-hover) !important;
}
/* 选中态：柔和底 + 左侧细指示条，两主题共用结构 */
.aside :deep(.el-menu-item.is-active) {
  background-color: var(--ch-sidebar-active-bg) !important;
  color: var(--ch-sidebar-text-active) !important;
  font-weight: 600;
  box-shadow: inset 2px 0 0 var(--ch-sidebar-active-bar);
}
.aside :deep(.el-sub-menu .el-menu-item) {
  background-color: transparent !important;
  height: 40px;
  line-height: 40px;
  margin-left: 14px;
  font-size: 13px;
}
.aside :deep(.el-sub-menu .el-menu-item:hover) {
  background-color: var(--ch-sidebar-hover-bg) !important;
}
.aside :deep(.el-sub-menu .el-menu-item.is-active) {
  background-color: var(--ch-sidebar-active-bg) !important;
  box-shadow: inset 2px 0 0 var(--ch-sidebar-active-bar);
}
.aside :deep(.el-sub-menu__icon-arrow) { color: var(--ch-sidebar-icon); }
.logo {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 0 14px 0 18px;
  color: var(--ch-sidebar-title);
  white-space: nowrap;
  border-bottom: 1px solid var(--ch-sidebar-border);
}
.brand { display: flex; align-items: center; gap: 11px; flex: 1; min-width: 0; }
.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  border-radius: 8px;
  background: linear-gradient(145deg, #6366f1, #4338ca);
  border: 1px solid rgba(255, 255, 255, 0.16);
  color: #fff;
  font-size: 15px;
  font-weight: 800;
  letter-spacing: -0.04em;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 2px 6px rgba(79, 70, 229, 0.25);
}
.brand-mark-collapsed { margin: 0 auto; }
.logo-text {
  font-size: 15px;
  font-weight: 650;
  letter-spacing: -0.01em;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--ch-sidebar-title);
}
.collapse-btn {
  font-size: 16px;
  cursor: pointer;
  color: var(--ch-sidebar-icon);
  flex: 0 0 auto;
  padding: 5px;
  border-radius: 6px;
  transition: color 0.16s, background-color 0.16s;
}
.collapse-btn:hover {
  color: var(--ch-sidebar-text-hover);
  background: var(--ch-sidebar-hover-bg);
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 64px;
  padding: 0 24px;
  border-bottom: 1px solid var(--ch-line-soft);
  background: var(--ch-header-bg);
  backdrop-filter: saturate(150%) blur(12px);
  -webkit-backdrop-filter: saturate(150%) blur(12px);
}
.title {
  position: relative;
  padding-left: 13px;
  font-size: 15px;
  font-weight: 650;
  color: var(--el-text-color-primary);
  letter-spacing: -0.01em;
}
.title::before {
  content: '';
  position: absolute;
  left: 0; top: 50%;
  transform: translateY(-50%);
  width: 3px; height: 15px;
  border-radius: 2px;
  background: var(--ch-accent);
}
.user {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--el-text-color-regular);
  font-size: 13px;
}
.user .el-tag { margin-left: 2px; }
.theme-toggle {
  font-size: 17px;
  cursor: pointer;
  color: var(--el-text-color-secondary);
  padding: 6px;
  border-radius: 7px;
  margin-right: 4px;
  transition: color 0.16s, background-color 0.16s;
}
.theme-toggle:hover {
  color: var(--ch-accent);
  background: var(--ch-surface-3);
}
.entity-label {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.notif-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-weight: 600;
  margin-bottom: 8px;
}
.notif-list {
  max-height: 320px;
  overflow: auto;
}
.notif-item {
  padding: 8px 6px;
  border-bottom: 1px solid #f0f2f5;
  cursor: pointer;
}
.notif-item.unread {
  background: var(--ch-accent-soft);
}
.notif-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.notif-content {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 2px;
}
.notif-time {
  font-size: 11px;
  color: var(--el-text-color-placeholder);
  margin-top: 2px;
}
.notif-empty {
  text-align: center;
  color: var(--el-text-color-placeholder);
  font-size: 12px;
  padding: 16px 0;
}
</style>
