<script setup>
import { onMounted, onUnmounted, ref, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { loadDirect, sendDirect } from '../api.js'

const route = useRoute()
const matchId = route.params.matchId

const messages = ref([])
const draft = ref('')
const error = ref('')
const sending = ref(false)
const bottom = ref(null)
let timer = null

onMounted(async () => {
  try {
    // 后端游标分页倒序返回（新的在前），界面按正序渲染
    messages.value = (await loadDirect(matchId)).reverse()
    scrollBottom()
  } catch (e) {
    error.value = e.message
  }
  timer = setInterval(poll, 4000) // MVP 短轮询；WebSocket 为 P1
})

onUnmounted(() => clearInterval(timer))

async function poll() {
  try {
    const fresh = (await loadDirect(matchId)).reverse()
    if (fresh.length !== messages.value.length) {
      messages.value = fresh
      scrollBottom()
    }
  } catch { /* 轮询失败静默，下轮再试 */ }
}

async function send() {
  const content = draft.value.trim()
  if (!content || sending.value) return
  sending.value = true
  error.value = ''
  draft.value = ''
  messages.value.push({ id: Date.now(), is_mine: true, content })
  scrollBottom()
  try {
    await sendDirect(matchId, content)
    messages.value = (await loadDirect(matchId)).reverse()
  } catch (e) {
    error.value = e.message
  } finally {
    sending.value = false
    scrollBottom()
  }
}

function scrollBottom() {
  nextTick(() => bottom.value?.scrollIntoView({ behavior: 'smooth', block: 'end' }))
}
</script>

<template>
  <main class="dm">
    <header class="bar">
      <router-link to="/meet" class="back">← 遇见</router-link>
      <p class="note">真人对话 · 请真诚相待</p>
    </header>

    <div class="stream">
      <p v-if="!messages.length && !error" class="empty">
        同频才走到这里。先说第一句吧。
      </p>
      <div v-for="m in messages" :key="m.id" :class="m.is_mine ? 'me-line' : 'them-line'">
        <p class="bubble">{{ m.content }}</p>
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div ref="bottom"></div>
    </div>

    <form class="composer" @submit.prevent="send">
      <textarea v-model="draft" rows="1" placeholder="说点什么……"
        @keydown.enter.exact.prevent="send"></textarea>
      <button type="submit" :disabled="sending || !draft.trim()">
        {{ sending ? '…' : '送出' }}
      </button>
    </form>
  </main>
</template>

<style scoped>
.dm {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.bar {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 14px 18px 10px;
  border-bottom: 1px solid var(--line);
}
.back { color: var(--amber); text-decoration: none; font-size: 14px; }
.note { color: var(--mist); font-size: 12.5px; margin: 0; }
.stream { flex: 1; overflow-y: auto; padding: 16px 18px 8px; }
.empty { color: var(--mist); text-align: center; font-family: var(--serif); margin-top: 48px; }
.me-line, .them-line { display: flex; margin: 10px 0; }
.me-line { justify-content: flex-end; }
.them-line { justify-content: flex-start; }
.bubble {
  margin: 0;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 10px 14px;
  max-width: 30ch;
  font-size: 15.5px;
}
.me-line .bubble { border-radius: 14px 14px 4px 14px; }
.them-line .bubble { border-radius: 14px 14px 14px 4px; }
.error { color: #d98f6a; font-size: 14px; margin: 8px 18px; }
.composer {
  display: flex;
  gap: 10px;
  padding: 10px 14px 14px;
  border-top: 1px solid var(--line);
}
.composer textarea { flex: 1; resize: none; max-height: 96px; }
.composer button {
  background: var(--amber);
  color: #191510;
  border-radius: 10px;
  padding: 0 18px;
  font-size: 15px;
  font-weight: 600;
}
.composer button:disabled { opacity: 0.4; }
</style>
