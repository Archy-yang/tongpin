<script setup>
import { onMounted, onUnmounted, ref, nextTick } from 'vue'
import { loadChatHistory, sendChatMessage } from '../api.js'

const messages = ref([]) // { id, sender, is_ai, content, created_at }
const draft = ref('')
const error = ref('')
const sending = ref(false)
const think = ref(false) // AI 思考中：三点呼吸
const bottom = ref(null)
const draftBox = ref(null)
let ws = null
let wsRetry = null

const greeting = { id: -1, is_ai: true, content: '我在。今晚想聊点什么，或者什么都不聊也行。' }

onMounted(async () => {
  try {
    const history = await loadChatHistory()
    // 后端游标分页倒序返回（新的在前），界面按正序渲染
    const ordered = history.reverse()
    messages.value = ordered.length ? ordered : [greeting]
  } catch (e) {
    messages.value = [greeting]
    error.value = '聊天记录加载失败，先聊聊看'
  }
  scrollBottom()
  connectWS()
})

onUnmounted(() => {
  clearTimeout(wsRetry)
  if (ws) { ws.onclose = null; ws.close() }
})

// WebSocket：接收 AI 主动开话（用户发消息仍走 REST）
function connectWS() {
  const token = localStorage.getItem('tongpin_token')
  if (!token) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ws = new WebSocket(`${proto}://${location.host}/api/v1/ws?token=${encodeURIComponent(token)}`)
  ws.onmessage = (ev) => {
    try {
      const frame = JSON.parse(ev.data)
      if (frame.type === 'chat_ai' && frame.message) {
        const m = frame.message
        if (!messages.value.some(x => x.id === m.id)) {
          messages.value.push(m)
          scrollBottom()
        }
      }
    } catch { /* 忽略坏帧 */ }
  }
  ws.onclose = () => { wsRetry = setTimeout(connectWS, 3000) }
}

// 输入框自动长高（上限与 CSS max-height 一致）
function grow() {
  const el = draftBox.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 92) + 'px'
}

async function send() {
  const content = draft.value.trim()
  if (!content || sending.value) return
  sending.value = true
  error.value = ''
  draft.value = ''
  if (draftBox.value) draftBox.value.style.height = 'auto'
  // 用户消息先上屏（与后端同序：先落自己的话）
  messages.value.push({ id: Date.now(), is_ai: false, content })
  scrollBottom()
  think.value = true // 三点呼吸：等 AI 想的时候
  try {
    const data = await sendChatMessage(content)
    // 替换掉乐观插入的那条，以服务端为准
    messages.value[messages.value.length - 1] = data.user_message
    messages.value.push(data.ai_message)
  } catch (e) {
    think.value = false
    // 免费额度用完（3002）：话还回输入框，不说丢
    if (e.code === 3002) draft.value = content
    error.value = e.message
  } finally {
    sending.value = false
    think.value = false
    scrollBottom()
  }
}

function scrollBottom() {
  nextTick(() => bottom.value?.scrollIntoView({ behavior: 'smooth', block: 'end' }))
}
</script>

<template>
  <main class="chat">
    <header class="pagehead">
      <p class="when">夜里 · 灯亮着</p>
      <h1>夜聊</h1>
    </header>

    <div class="stream">
      <template v-for="m in messages" :key="m.id">
        <!-- AI 消息不用气泡：琥珀标记 + 灯下低声（AI 标识是产品红线） -->
        <div v-if="m.is_ai" class="ai-line msg-in">
          <span class="ai-mark" aria-label="AI">AI</span>
          <p class="ai-text">{{ m.content }}</p>
        </div>
        <div v-else class="me-line msg-in">
          <p class="note-bubble">{{ m.content }}</p>
        </div>
      </template>
      <!-- AI 思考中 -->
      <div v-if="think" class="ai-line" aria-live="polite">
        <span class="ai-mark">AI</span>
        <span class="typing-dots"><i></i><i></i><i></i></span>
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div ref="bottom"></div>
    </div>

    <form class="composer" @submit.prevent="send">
      <textarea
        ref="draftBox"
        v-model="draft"
        rows="1"
        placeholder="说点什么……"
        @input="grow"
        @keydown.enter.exact.prevent="send"
      ></textarea>
      <button class="btn-lamp" type="submit" :disabled="sending || !draft.trim()">送出</button>
    </form>
  </main>
</template>

<style scoped>
.chat {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.stream {
  flex: 1;
  overflow-y: auto;
  padding: 8px 20px 10px;
}
.ai-line {
  margin: 16px 0;
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.ai-mark {
  flex: none;
  margin-top: 5px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 1px solid var(--lamp);
  color: var(--lamp);
  font-size: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.ai-text {
  margin: 0;
  color: var(--paper);
  font-family: var(--serif);
  font-size: 16px;
  line-height: 1.8;
  max-width: 30ch;
}
.me-line {
  display: flex;
  justify-content: flex-end;
  margin: 14px 0;
}
/* 暖纸便签：整个夜里唯一亮着的纸 */
.note-bubble {
  margin: 0;
  background: var(--note);
  color: var(--note-ink);
  border-radius: 16px 16px 4px 16px;
  padding: 10px 14px;
  max-width: 30ch;
  font-size: 15px;
  line-height: 1.6;
  box-shadow: 0 2px 14px rgba(231, 223, 204, .12);
}
.error {
  color: #d98f6a;
  font-size: 14px;
  margin: 8px 4px;
}
.composer {
  display: flex;
  gap: 10px;
  padding: 10px 16px 14px;
  border-top: 1px solid var(--line);
  background: rgba(13, 15, 22, .6);
}
.composer textarea {
  flex: 1;
  resize: none;
  max-height: 92px;
  line-height: 1.5;
}
.composer .btn-lamp {
  padding: 0 20px;
  font-size: 15px;
}
</style>
