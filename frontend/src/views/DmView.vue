<script setup>
import { onMounted, onUnmounted, ref, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { loadDirect, sendDirect, sendFeedback } from '../api.js'

const route = useRoute()
const matchId = route.params.matchId

const messages = ref([])
const draft = ref('')
const error = ref('')
const sending = ref(false)
const bottom = ref(null)
const feedback = ref('') // 'like' | 'mismatch' | ''（已反馈）
const live = ref(false)  // WebSocket 是否在线
let timer = null
let ws = null
let wsRetry = null

onMounted(async () => {
  try {
    // 后端游标分页倒序返回（新的在前），界面按正序渲染
    messages.value = (await loadDirect(matchId)).reverse()
    scrollBottom()
  } catch (e) {
    error.value = e.message
  }
  connectWS()
  // 兜底轮询：WS 在线时降频，离线时 4 秒
  timer = setInterval(poll, 4000)
})

onUnmounted(() => {
  clearInterval(timer)
  clearTimeout(wsRetry)
  if (ws) { ws.onclose = null; ws.close() }
})

function connectWS() {
  const token = localStorage.getItem('tongpin_token')
  if (!token) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ws = new WebSocket(`${proto}://${location.host}/api/v1/ws?token=${encodeURIComponent(token)}`)
  ws.onopen = () => { live.value = true }
  ws.onmessage = (ev) => {
    try {
      const frame = JSON.parse(ev.data)
      if (frame.type === 'dm' && String(frame.match_id) === String(matchId) && frame.message) {
        append(frame.message)
      }
    } catch { /* 忽略坏帧 */ }
  }
  ws.onclose = () => {
    live.value = false
    wsRetry = setTimeout(connectWS, 3000) // 断线重连
  }
}

function append(msg) {
  if (messages.value.some(m => m.id === msg.id)) return // REST/WS 双通道去重
  messages.value.push(msg)
  scrollBottom()
}

async function poll() {
  // WS 在线时只做低频校准；离线时承担兜底
  if (live.value && messages.value.length && Math.random() > 0.25) return
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
  messages.value.push({ id: `tmp-${Date.now()}`, is_mine: true, content })
  scrollBottom()
  try {
    await sendDirect(matchId, content)
    // 整表校准：清理乐观条目，WS 回显 / 兜底轮询的重复也一并消除
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

async function rate(direction) {
  if (feedback.value) return
  feedback.value = direction
  try {
    await sendFeedback(matchId, direction)
  } catch (e) {
    error.value = e.message
  }
}
</script>

<template>
  <main class="dm">
    <header class="bar">
      <router-link to="/meet" class="back">← 遇见</router-link>
      <div class="rate" v-if="!feedback">
        <button class="rate-btn" @click="rate('like')" title="聊得来">聊得来</button>
        <button class="rate-btn" @click="rate('mismatch')" title="不合适">不合适</button>
      </div>
      <p class="note" v-else-if="feedback === 'like'">同频，真好</p>
      <p class="note" v-else>收到，之后会少推这样的人</p>
    </header>
    <p class="ws-note">{{ live ? '实时连接中' : '连接中……' }}</p>

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
.on { color: #8fae7e; }
.off { color: var(--mist); }
.ws-note {
  margin: 0;
  padding: 4px 18px 0;
  color: var(--mist);
  font-size: 11.5px;
  text-align: right;
}
.rate { display: flex; gap: 8px; }
.rate-btn {
  background: none;
  border: 1px solid var(--line);
  color: var(--mist);
  border-radius: 8px;
  padding: 4px 10px;
  font-size: 12.5px;
}
.rate-btn:hover { border-color: var(--amber); color: var(--amber); }
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
