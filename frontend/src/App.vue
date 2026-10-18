<script setup>
import { onMounted, ref } from 'vue'
import { ensureLogin, reportOpen } from './api.js'

const user = ref(null)
const ready = ref(false)

onMounted(async () => {
  try {
    user.value = await ensureLogin()
  } catch (e) {
    // 登录失败也放行到界面，各页自行处理接口错误
    console.warn('auto login failed:', e.message)
  } finally {
    ready.value = true
  }
  // 埋点：打开应用（留存统计用），尽力而为
  reportOpen().catch(() => {})
})
</script>

<template>
  <div v-if="ready" class="page-enter app-shell">
    <router-view v-slot="{ Component }">
      <transition name="tab-fade" mode="out-in">
        <component :is="Component" />
      </transition>
    </router-view>
    <nav class="tabbar">
      <router-link to="/chat" class="tab" aria-label="夜聊">
        <svg viewBox="0 0 24 24"><path d="M19 14.5A8 8 0 0 1 9.5 5 8 8 0 1 0 19 14.5Z"/></svg>
        <span>夜聊</span>
      </router-link>
      <router-link to="/style" class="tab" aria-label="同频卡">
        <svg viewBox="0 0 24 24"><rect x="4" y="5" width="16" height="14" rx="3"/><path d="M8 10h8M8 14h5"/></svg>
        <span>同频卡</span>
      </router-link>
      <router-link to="/meet" class="tab" aria-label="遇见">
        <svg viewBox="0 0 24 24"><path d="M21 3 3.5 10.5 10 13l2.5 6.5L21 3Z"/><path d="M10 13 21 3"/></svg>
        <span>遇见</span>
      </router-link>
      <router-link to="/me" class="tab" aria-label="我的">
        <svg viewBox="0 0 24 24"><circle cx="12" cy="8.5" r="3.5"/><path d="M5 19.5c1.5-3.5 4-5 7-5s5.5 1.5 7 5"/></svg>
        <span>我的</span>
      </router-link>
    </nav>
  </div>
</template>

<style scoped>
.app-shell {
  flex: 1;
  min-height: 0; /* 允许被压缩，内部区域自行滚动，tabbar 始终贴底 */
  display: flex;
  flex-direction: column;
}

/* tab 切换：160ms 淡入，不打断手 */
.tab-fade-enter-active, .tab-fade-leave-active { transition: opacity .16s ease, transform .16s ease; }
.tab-fade-enter-from { opacity: 0; transform: translateY(8px); }
.tab-fade-leave-to { opacity: 0; }

.tabbar {
  flex: none;
  display: flex;
  border-top: 1px solid var(--line);
  background: rgba(13, 15, 22, .82);
  backdrop-filter: blur(14px);
  padding: 6px 8px calc(10px + env(safe-area-inset-bottom, 0));
}
.tab {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  padding: 6px 0 2px;
  color: var(--moon-dim);
  text-decoration: none;
  font-size: 11px;
  letter-spacing: .06em;
  position: relative;
  transition: color .16s;
}
.tab svg {
  width: 21px;
  height: 21px;
  stroke: currentColor;
  fill: none;
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.tab::before { /* active 灯条 */
  content: '';
  position: absolute;
  top: -7px;
  width: 16px;
  height: 2px;
  border-radius: 2px;
  background: transparent;
  transition: background .16s;
}
.tab.router-link-active { color: var(--lamp); }
.tab.router-link-active::before {
  background: var(--lamp);
  box-shadow: 0 0 8px rgba(226, 168, 101, .7);
}
.tab:active { transform: scale(.94); }
</style>
