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
    <router-view />
    <nav class="tabbar">
      <router-link to="/chat" class="tab">夜聊</router-link>
      <router-link to="/style" class="tab">同频卡</router-link>
      <router-link to="/meet" class="tab">遇见</router-link>
      <router-link to="/me" class="tab">我的</router-link>
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
.tabbar {
  display: flex;
  border-top: 1px solid var(--line);
  background: var(--ink);
  padding-bottom: env(safe-area-inset-bottom, 0);
}
.tab {
  flex: 1;
  text-align: center;
  padding: 14px 0 12px;
  color: var(--mist);
  text-decoration: none;
  font-size: 15px;
  letter-spacing: 0.05em;
}
.tab.router-link-active {
  color: var(--amber);
}
</style>
