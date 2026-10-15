<script setup>
import { onMounted, ref, nextTick } from 'vue'
import { loadChatHistory, sendChatMessage } from '../api.js'

const messages = ref([]) // { id, sender, is_ai, content, created_at }
const draft = ref('')
const error = ref('')
const sending = ref(false)
const bottom = ref(null)

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
})

async function send() {
  const content = draft.value.trim()
  if (!content || sending.value) return
  sending.value = true
  error.value = ''
  draft.value = ''
  // 用户消息先上屏（与后端同序：先落自己的话）
  messages.value.push({ id: Date.now(), is_ai: false, content })
  scrollBottom()
  try {
    const data = await sendChatMessage(content)
    // 替换掉乐观插入的那条，以服务端为准
    messages.value[messages.value.length - 1] = data.user_message
    messages.value.push(data.ai_message)
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
  <main class="chat">
    <div class="stream">
      <template v-for="m in messages" :key="m.id">
        <!-- AI 消息不用气泡：琥珀标记 + 低声正文（AI 标识是产品红线） -->
        <div v-if="m.is_ai" class="ai-line">
          <span class="ai-mark" aria-label="AI">AI</span>
          <p class="ai-text">{{ m.content }}</p>
        </div>
        <div v-else class="me-line">
          <p class="me-bubble">{{ m.content }}</p>
        </div>
      </template>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div ref="bottom"></div>
    </div>

    <form class="composer" @submit.prevent="send">
      <textarea
        v-model="draft"
        rows="1"
        placeholder="说点什么……"
        @keydown.enter.exact.prevent="send"
      ></textarea>
      <button type="submit" :disabled="sending || !draft.trim()">
        {{ sending ? '…' : '送出' }}
      </button>
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
  padding: 20px 18px 10px;
}
.ai-line {
  margin: 18px 0;
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.ai-mark {
  flex: none;
  margin-top: 4px;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 1px solid var(--amber);
  color: var(--amber);
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.ai-text {
  margin: 0;
  color: var(--paper);
  font-family: var(--serif);
  font-size: 16.5px;
  line-height: 1.75;
  max-width: 34ch;
}
.me-line {
  display: flex;
  justify-content: flex-end;
  margin: 14px 0;
}
.me-bubble {
  margin: 0;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 14px 14px 4px 14px;
  padding: 10px 14px;
  max-width: 30ch;
  font-size: 15.5px;
}
.error {
  color: #d98f6a;
  font-size: 14px;
  margin: 8px 18px;
}
.composer {
  display: flex;
  gap: 10px;
  padding: 10px 14px 14px;
  border-top: 1px solid var(--line);
}
.composer textarea {
  flex: 1;
  resize: none;
  max-height: 96px;
}
.composer button {
  background: var(--amber);
  color: #191510;
  border-radius: 10px;
  padding: 0 18px;
  font-size: 15px;
  font-weight: 600;
}
.composer button:disabled {
  opacity: 0.4;
  cursor: default;
}
</style>
