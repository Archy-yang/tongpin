<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  getOptIn, optIn, loadSuggestions, createContactRequest,
  listRequests, approveRequest, rejectRequest, revokeRequest,
  createIcebreak, myIcebreaks, approveIcebreak, declineIcebreak,
} from '../api.js'

const loading = ref(true)
const opted = ref(null)
const view = ref(null)
const inbox = ref([])
const outbox = ref([])
const icebreaks = ref([]) // 按 match_id 索引的暖场会话
const error = ref('')
const needBind = ref(false) // 进池被邮箱门槛拦下
const joining = ref(false)
const busyId = ref(null)
let pollTimer = null

const form = ref({
  relationship_intent: 'serious_but_slow',
  age_pref_min: null,
  age_pref_max: null,
  accept_real_contact: true,
})

const intents = [
  { v: 'casual', label: '随便聊聊' },
  { v: 'friends', label: '先做朋友' },
  { v: 'serious_but_slow', label: '认真但慢慢来' },
  { v: 'serious', label: '认真关系' },
]

const icebreakByMatch = computed(() => {
  const map = {}
  for (const s of icebreaks.value) map[s.match_id] = s
  return map
})

const hasGenerating = computed(() => icebreaks.value.some(s => s.status === 'generating'))

onMounted(async () => {
  await refresh()
  pollTimer = setInterval(async () => {
    if (hasGenerating.value) await loadIcebreaks()
  }, 5000)
})

onUnmounted(() => clearInterval(pollTimer))

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    opted.value = await getOptIn()
    if (opted.value) {
      await Promise.all([loadAll(), loadRequests(), loadIcebreaks()])
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function loadAll() { view.value = await loadSuggestions() }

async function loadRequests() {
  const [i, o] = await Promise.all([listRequests('inbox'), listRequests('outbox')])
  inbox.value = i
  outbox.value = o
}

async function loadIcebreaks() { icebreaks.value = await myIcebreaks() }

async function join() {
  joining.value = true
  error.value = ''
  try {
    await optIn({
      relationship_intent: form.value.relationship_intent,
      age_pref_min: form.value.age_pref_min || null,
      age_pref_max: form.value.age_pref_max || null,
      accept_real_contact: form.value.accept_real_contact,
    })
    await refresh()
  } catch (e) {
    if (e.message === 'style profile not ready') {
      error.value = '同频卡还没成形，先去夜聊页多聊几句'
    } else if (e.message.includes('绑定邮箱')) {
      error.value = e.message
      needBind.value = true // 引导去「我的」
    } else {
      error.value = e.message
    }
  } finally {
    joining.value = false
  }
}

async function act(fn, id) {
  busyId.value = id
  error.value = ''
  try {
    await fn(id)
    await Promise.all([loadRequests(), loadAll(), loadIcebreaks()])
  } catch (e) {
    error.value = e.message
  } finally {
    busyId.value = null
  }
}

const approvedOutbox = () => outbox.value.filter(r => r.status === 'approved')
const pendingInbox = () => inbox.value.filter(r => r.status === 'pending')
const pendingOutbox = () => outbox.value.filter(r => r.status === 'pending')
const approvedInbox = () => inbox.value.filter(r => r.status === 'approved')

// 待我同意的暖场请求（我是 to_user 且 pending）
const incomingIcebreaks = () => icebreaks.value.filter(s => s.status === 'pending' && s.to_user !== s.from_user && !outbox.value.some(r => r.match_id === s.match_id) && !inbox.value.some(r => r.match_id === s.match_id))
const activeIcebreaks = () => icebreaks.value.filter(s => ['generating', 'ready', 'failed'].includes(s.status))
</script>

<template>
  <main class="meet">
    <header class="pagehead">
      <p class="when">夜里 · 宁缺毋滥</p>
      <h1>遇见</h1>
      <p class="sub">把同频卡拿去，认识说话方式相像的人</p>
    </header>

    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <router-link v-if="needBind" to="/me" class="bind-link">去「我的」绑定邮箱 →</router-link>
    <p v-if="loading" class="hint">…</p>

    <!-- 未进入匹配池 -->
    <section v-else-if="!opted" class="join">
      <p class="join-lede">
        不看照片，不填标签。
        你的同频卡会替你先说话——对方只会看到你们的契合点，看不到你。
      </p>
      <form class="join-form" @submit.prevent="join">
        <label>
          <span>你想找什么</span>
          <select v-model="form.relationship_intent">
            <option v-for="i in intents" :key="i.v" :value="i.v">{{ i.label }}</option>
          </select>
        </label>
        <div class="age-row">
          <label>
            <span>年龄从</span>
            <select v-model="form.age_pref_min">
              <option :value="null">不限</option>
              <option v-for="n in [18, 22, 25, 28, 31, 35, 40]" :key="n" :value="n">{{ n }}</option>
            </select>
          </label>
          <label>
            <span>到</span>
            <select v-model="form.age_pref_max">
              <option :value="null">不限</option>
              <option v-for="n in [22, 25, 28, 31, 35, 40, 50]" :key="n" :value="n">{{ n }}</option>
            </select>
          </label>
        </div>
        <label class="check">
          <input type="checkbox" v-model="form.accept_real_contact" />
          <span>允许对方请求和我真人聊天（随时可撤回）</span>
        </label>
        <button type="submit" :disabled="joining">{{ joining ? '…' : '进入匹配池' }}</button>
      </form>
    </section>

    <!-- 已进入 -->
    <template v-else>
      <!-- 暖场：待我同意 -->
      <section v-if="incomingIcebreaks().length" class="reqs">
        <h2>暖场请求</h2>
        <article v-for="s in incomingIcebreaks()" :key="'ib' + s.id" class="req">
          <p class="who">对方想先让两个 AI 替你们试试聊不聊得来</p>
          <div class="acts">
            <button class="primary" :disabled="busyId === s.id" @click="act(approveIcebreak, s.id)">让 AI 试试</button>
            <button class="ghost" :disabled="busyId === s.id" @click="act(declineIcebreak, s.id)">不必了</button>
          </div>
        </article>
      </section>

      <!-- 暖场：进行中 / 完成 -->
      <section v-if="activeIcebreaks().length" class="reqs">
        <h2>AI 暖场</h2>
        <article v-for="s in activeIcebreaks()" :key="'ibg' + s.id" class="req">
          <template v-if="s.status === 'generating'">
            <p class="who">两个 AI 正在替你们试聊……</p>
            <p class="ice-note">大约十几秒，好了会自己亮出来</p>
          </template>
          <template v-else-if="s.status === 'ready' && s.summary">
            <p class="who">AI 试聊后的观察</p>
            <ul class="hl">
              <li v-for="(p, i) in s.summary.common_points" :key="'c' + i">{{ p }}</li>
            </ul>
            <p class="ice-label">可以聊</p>
            <ul class="hl">
              <li v-for="(t, i) in s.summary.topics" :key="'t' + i">{{ t }}</li>
            </ul>
            <p class="ice-label">小心点</p>
            <ul class="hl">
              <li v-for="(c, i) in s.summary.cautions" :key="'w' + i">{{ c }}</li>
            </ul>
            <p class="ice-label">开场建议</p>
            <p class="opener">{{ s.summary.opener }}</p>
            <button class="primary" :disabled="busyId === s.match_id" @click="act(createContactRequest, s.match_id)">说说话</button>
          </template>
          <template v-else-if="s.status === 'failed'">
            <p class="who">暖场没弄成，但不影响你们直接聊</p>
            <button class="primary" :disabled="busyId === s.match_id" @click="act(createContactRequest, s.match_id)">说说话</button>
          </template>
        </article>
      </section>

      <!-- 契合提示 -->
      <section v-if="view?.pool_status === 'warming_up'" class="warming">
        <p class="warm-num">{{ view.active_users }}</p>
        <p class="warm-text">
          个人已经在这里等了。<br />等人数再多一些，契合提示就会亮起来——宁缺毋滥。
        </p>
      </section>

      <section v-else-if="view?.suggestions?.length" class="sugs">
        <h2>和你同频的人</h2>
        <article v-for="s in view.suggestions" :key="s.match_id" class="sug">
          <ul class="hl">
            <li v-for="(h, idx) in s.highlights" :key="idx">{{ h }}</li>
          </ul>
          <div class="acts" v-if="!icebreakByMatch[s.match_id] && !outbox.some(r => r.match_id === s.match_id) && !approvedOutbox().some(r => r.match_id === s.match_id)">
            <button class="primary" :disabled="busyId === s.match_id" @click="act(createContactRequest, s.match_id)">说说话</button>
            <button class="ghost" :disabled="busyId === s.match_id" @click="act(createIcebreak, s.match_id)">先让 AI 试试</button>
          </div>
          <p v-else-if="icebreakByMatch[s.match_id]" class="sent">暖场里见</p>
          <p v-else class="sent">已递出纸条，等对方回应</p>
        </article>
      </section>

      <section v-else-if="!activeIcebreaks().length && !incomingIcebreaks().length" class="none">
        <p>暂时没有契合的人。你的同频卡会一直替你留意。</p>
      </section>

      <!-- 收到的真人接触请求 -->
      <section v-if="pendingInbox().length || approvedInbox().length" class="reqs">
        <h2>递来的纸条</h2>
        <article v-for="r in pendingInbox()" :key="r.id" class="req">
          <p class="who">{{ r.counterpart_nickname }} 想和你说说话</p>
          <div class="acts">
            <button class="primary" :disabled="busyId === r.id" @click="act(approveRequest, r.id)">见</button>
            <button class="ghost" :disabled="busyId === r.id" @click="act(rejectRequest, r.id)">不见</button>
          </div>
        </article>
        <article v-for="r in approvedInbox()" :key="'a' + r.id" class="req">
          <p class="who">和 {{ r.counterpart_nickname }} 聊上了</p>
          <router-link class="primary link" :to="`/dm/${r.match_id}`">去聊天</router-link>
        </article>
      </section>

      <!-- 发出的请求 -->
      <section v-if="approvedOutbox().length" class="reqs">
        <h2>聊上了</h2>
        <article v-for="r in approvedOutbox()" :key="r.id" class="req">
          <p class="who">和 {{ r.counterpart_nickname }}</p>
          <router-link class="primary link" :to="`/dm/${r.match_id}`">去聊天</router-link>
        </article>
      </section>
      <section v-else-if="pendingOutbox().length" class="reqs">
        <h2>递出去的纸条</h2>
        <article v-for="r in pendingOutbox()" :key="r.id" class="req">
          <p class="who">{{ r.counterpart_nickname }} · 等待回应</p>
          <button class="ghost" :disabled="busyId === r.id" @click="act(revokeRequest, r.id)">收回</button>
        </article>
      </section>
    </template>
  </main>
</template>

<style scoped>
.meet { flex: 1; overflow-y: auto; padding: 0 20px 24px; }
.sub { margin: 2px 0 0; color: var(--moon); font-size: 12.5px; }
.error { color: #d98f6a; font-size: 14px; }
.bind-link { color: var(--lamp); font-size: 14px; text-decoration: none; }
.hint { color: var(--moon); }

.join { margin-top: 28px; }
.join-lede {
  font-family: var(--serif);
  font-size: 16px;
  line-height: 1.8;
  max-width: 30ch;
}
.join-form { display: flex; flex-direction: column; gap: 14px; margin-top: 20px; }
.join-form label { display: flex; flex-direction: column; gap: 6px; }
.join-form span { color: var(--moon); font-size: 14px; }
select { accent-color: var(--lamp); font-family: inherit; background: var(--panel); color: var(--paper); border: 1px solid var(--line); border-radius: 10px; padding: 10px 12px; font-size: 15px; }
.age-row { display: flex; gap: 14px; }
.age-row label { flex: 1; }
.check { flex-direction: row !important; align-items: center; gap: 8px !important; }
.check span { color: var(--moon); font-size: 13.5px; }
.join-form button { margin-top: 8px; background: var(--lamp); color: var(--lamp-ink); border-radius: 10px; padding: 11px 0; font-size: 15px; font-weight: 600; }
.join-form button:disabled { opacity: 0.5; }

.warming { margin-top: 64px; text-align: center; }
.warm-num { font-family: var(--serif); font-size: 56px; margin: 0; }
.warm-text { color: var(--moon); font-size: 15px; line-height: 1.8; margin-top: 10px; }
.none { margin-top: 48px; text-align: center; color: var(--moon); font-size: 15px; }

h2 { font-size: 12px; color: var(--moon); font-weight: normal; letter-spacing: .1em; margin: 26px 0 10px; }
.sug, .req { border: 1px solid var(--line); border-radius: 18px; background: var(--panel); padding: 18px 16px; margin-bottom: 12px; }
.hl { margin: 0 0 10px; padding: 0; list-style: none; }
.hl li { font-family: var(--serif); font-size: 15px; line-height: 1.7; padding: 3px 0 3px 16px; position: relative; }
.hl li::before {
  content: '';
  position: absolute;
  left: 0;
  top: 13px;
  width: 6px;
  height: 1px;
  background: var(--lamp);
}
.ice-label { margin: 10px 0 2px; color: var(--moon); font-size: 12px; letter-spacing: .08em; }
.opener { margin: 2px 0 12px; font-family: var(--serif); font-size: 15px; line-height: 1.7; border-left: 2px solid var(--lamp); padding-left: 12px; }
.ice-note { margin: 0; color: var(--moon); font-size: 13.5px; }
.primary { background: var(--lamp); color: var(--lamp-ink); border-radius: 11px; padding: 9px 18px; font-size: 14.5px; font-weight: 600; text-decoration: none; display: inline-block; }
.primary:disabled { opacity: 0.5; }
.link { padding: 8px 16px; }
.ghost { background: none; border: 1px solid var(--line); color: var(--moon); border-radius: 11px; padding: 8px 16px; font-size: 14px; }
.sent { color: var(--moon); font-size: 14px; margin: 0; }
.who { margin: 0 0 10px; font-family: var(--serif); font-size: 15.5px; }
.acts { display: flex; gap: 10px; align-items: center; }
</style>
