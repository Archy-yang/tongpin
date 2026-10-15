<script setup>
import { onMounted, ref } from 'vue'
import { loadStyleProfile, deleteStyleProfile } from '../api.js'

const view = ref(null) // { status, message_count, profile, confidence, version }
const error = ref('')
const confirmingDelete = ref(false)
const deleting = ref(false)

const MATURE = 50 // 与后端 matureMessageThreshold 一致

// 枚举值 → 中文
const pace = { fast: '快节奏', medium: '中速', slow: '慢热' }
const replyLen = { short: '简短', short_to_medium: '简短到适中', medium: '适中', long: '展开说' }
const emoji = { none: '不用表情', low: '少量', medium: '适中', high: '高频' }
const formality = { formal: '正式', neutral: '中性', casual: '随意' }
const expression = { direct: '直接', reserved: '含蓄' }
const tone = { positive: '积极', calm: '平和', melancholic: '低沉' }
const vuln = { fast: '很快交心', medium: '循序渐进', slow: '慢慢来' }
const conflict = { avoidant: '回避', rational: '讲道理', emotional: '情绪先行', compromising: '求和' }
const sens = { high: '高', medium: '中', low: '低' }
const intent = {
  casual: '随便聊聊', friends: '先做朋友',
  serious: '认真关系', serious_but_slow: '认真但慢慢来',
}
const hours = { morning: '清晨', afternoon: '午后', evening: '夜晚', late_night: '深夜' }
const topics = {
  music: '音乐', movies: '电影', books: '读书', games: '游戏', sports: '运动',
  travel: '旅行', food: '美食', philosophy: '哲学', technology: '科技', art: '艺术',
  life: '生活', work: '工作', career: '职业', relationships: '人际关系', emotions: '情感',
  health: '健康', finance: '财经', family: '家庭', study: '学习', late_night_thoughts: '深夜漫谈',
}

function fmt(map, v) { return v ? (map[v] ?? v) : '——' }
function fmtTopics(list) {
  return (list ?? []).map(t => topics[t] ?? t).join(' · ') || '——'
}

onMounted(load)

async function load() {
  error.value = ''
  confirmingDelete.value = false
  try {
    view.value = await loadStyleProfile()
  } catch (e) {
    error.value = e.message
  }
}

async function remove() {
  if (!confirmingDelete.value) {
    confirmingDelete.value = true
    return
  }
  deleting.value = true
  try {
    await deleteStyleProfile()
    await load()
  } catch (e) {
    error.value = e.message
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <main class="style-page">
    <header class="head">
      <h1>同频卡</h1>
      <p class="sub">从你和 AI 的对话里长出来的画像</p>
    </header>

    <p v-if="error" class="error" role="alert">{{ error }}</p>

    <!-- 画像未成形 -->
    <section v-if="view && view.status === 'collecting'" class="collecting">
      <p class="collect-num">{{ view.message_count ?? 0 }}<span> 句</span></p>
      <p class="collect-text">
        你们已经聊过这么多。大约在第 {{ MATURE }} 句前后，
        你的同频卡会慢慢成形——不用填任何标签，说话就好。
      </p>
    </section>

    <!-- 画像就绪 -->
    <section v-else-if="view && view.status === 'ready'" class="card">
      <p class="meta">第 {{ view.version }} 版 · 把握 {{ Math.round(view.confidence * 100) }}%</p>

      <dl class="grid">
        <div class="row"><dt>沟通节奏</dt><dd>{{ fmt(pace, view.profile.communication_style?.pace) }}</dd></div>
        <div class="row"><dt>说话长度</dt><dd>{{ fmt(replyLen, view.profile.communication_style?.reply_length) }}</dd></div>
        <div class="row"><dt>表达方式</dt><dd>{{ fmt(expression, view.profile.emotional_style?.expression) }} · {{ fmt(tone, view.profile.emotional_style?.tone) }}</dd></div>
        <div class="row"><dt>交心速度</dt><dd>{{ fmt(vuln, view.profile.emotional_style?.vulnerability_speed) }}</dd></div>
        <div class="row"><dt>起冲突时</dt><dd>{{ fmt(conflict, view.profile.conflict_style) }}</dd></div>
        <div class="row"><dt>隐私敏感</dt><dd>{{ fmt(sens, view.profile.boundary?.privacy_sensitivity) }}</dd></div>
        <div class="row"><dt>关系意图</dt><dd>{{ fmt(intent, view.profile.relationship_intent) }}</dd></div>
        <div class="row"><dt>常在时段</dt><dd>{{ (view.profile.active_hours ?? []).map(h => fmt(hours, h)).join('、') || '——' }}</dd></div>
      </dl>

      <div v-if="view.profile.topic_preferences?.length" class="topics">
        <h2>常聊的话题</h2>
        <p class="topic-list">{{ fmtTopics(view.profile.topic_preferences) }}</p>
      </div>

      <p class="note">
        这张卡用来找和你同频的人。只有匹配时使用，
        对方看到的只是契合点，看不到卡片本身。
      </p>

      <div class="danger-zone">
        <button class="ghost" :disabled="deleting" @click="remove">
          {{ confirmingDelete ? '再按一次确认删除' : '删除这张卡' }}
        </button>
        <p v-if="confirmingDelete" class="danger-hint">删除后画像与向量立即清除，分析停止。</p>
      </div>
    </section>
  </main>
</template>

<style scoped>
.style-page {
  flex: 1;
  overflow-y: auto;
  padding: 28px 22px 30px;
}
.head h1 {
  font-family: var(--serif);
  font-weight: 600;
  font-size: 26px;
  margin: 0;
}
.sub {
  margin: 4px 0 0;
  color: var(--mist);
  font-size: 14px;
}
.error { color: #d98f6a; font-size: 14px; }

.collecting {
  margin-top: 64px;
  text-align: center;
}
.collect-num {
  font-family: var(--serif);
  font-size: 56px;
  margin: 0;
  color: var(--paper);
}
.collect-num span { font-size: 18px; color: var(--mist); }
.collect-text {
  margin: 12px auto 0;
  max-width: 26ch;
  color: var(--mist);
  font-size: 15px;
}

.card {
  margin-top: 24px;
  border: 1px solid var(--line);
  border-radius: 16px;
  background: var(--panel);
  padding: 20px 18px;
}
.meta {
  margin: 0 0 12px;
  color: var(--amber);
  font-size: 13px;
}
.grid { margin: 0; }
.row {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 9px 0;
  border-bottom: 1px solid var(--line);
}
.row:last-child { border-bottom: none; }
.row dt { color: var(--mist); font-size: 14.5px; }
.row dd {
  margin: 0;
  font-size: 14.5px;
  text-align: right;
  font-family: var(--serif);
}
.topics h2 {
  font-size: 14.5px;
  color: var(--mist);
  font-weight: normal;
  margin: 14px 0 4px;
}
.topic-list {
  margin: 0;
  font-family: var(--serif);
  font-size: 15px;
}
.note {
  margin: 16px 0 0;
  color: var(--mist);
  font-size: 13px;
  line-height: 1.7;
}
.danger-zone { margin-top: 18px; }
.ghost {
  background: none;
  border: 1px solid var(--line);
  color: var(--mist);
  border-radius: 10px;
  padding: 8px 14px;
  font-size: 14px;
}
.ghost:hover { border-color: #d98f6a; color: #d98f6a; }
.danger-hint { color: #d98f6a; font-size: 13px; margin: 8px 0 0; }
</style>
