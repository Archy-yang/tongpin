<script setup>
import { onMounted, ref } from 'vue'
import {
  getOptIn, optIn, loadSuggestions, createContactRequest,
  listRequests, approveRequest, rejectRequest, revokeRequest,
} from '../api.js'

const loading = ref(true)
const opted = ref(null)   // opt-in 状态（null = 未进入）
const view = ref(null)    // suggestions 响应
const inbox = ref([])
const outbox = ref([])
const error = ref('')
const joining = ref(false)
const busyId = ref(null)

const form = ref({
  relationship_intent: 'serious_but_slow',
  age_pref_min: null,
  age_pref_max: null,
  accept_real_contact: true,
})

const intents = [
  { v: 'casual', label: '随便聊聊' },
  { v: 'friends', label: '先做朋友' },
  { v: 'friends_slow', label: '' }, // 占位防误用
  { v: 'serious_but_slow', label: '认真但慢慢来' },
  { v: 'serious', label: '认真关系' },
].filter(i => i.label)

onMounted(refresh)

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    opted.value = await getOptIn()
    if (opted.value) {
      await Promise.all([loadAll(), loadRequests()])
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function loadAll() {
  view.value = await loadSuggestions()
}

async function loadRequests() {
  const [i, o] = await Promise.all([listRequests('inbox'), listRequests('outbox')])
  inbox.value = i
  outbox.value = o
}

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
    error.value = e.message === 'style profile not ready'
      ? '同频卡还没成形，先去夜聊页多聊几句'
      : e.message
  } finally {
    joining.value = false
  }
}

async function reach(matchId) {
  busyId.value = matchId
  error.value = ''
  try {
    await createContactRequest(matchId)
    await loadRequests()
    await loadAll()
  } catch (e) {
    error.value = e.message
  } finally {
    busyId.value = null
  }
}

async function act(fn, id) {
  busyId.value = id
  error.value = ''
  try {
    await fn(id)
    await Promise.all([loadRequests(), loadAll()])
  } catch (e) {
    error.value = e.message
  } finally {
    busyId.value = null
  }
}

const approvedInbox = () => inbox.value.filter(r => r.status === 'approved')
const pendingInbox = () => inbox.value.filter(r => r.status === 'pending')
const pendingOutbox = () => outbox.value.filter(r => r.status === 'pending')
const approvedOutbox = () => outbox.value.filter(r => r.status === 'approved')
</script>

<template>
  <main class="meet">
    <header class="head">
      <h1>遇见</h1>
      <p class="sub">把同频卡拿去，认识说话方式相像的人</p>
    </header>

    <p v-if="error" class="error" role="alert">{{ error }}</p>
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
          <button
            v-if="!outbox.some(r => r.match_id === s.match_id) && !approvedOutbox().some(r => r.match_id === s.match_id)"
            class="primary" :disabled="busyId === s.match_id" @click="reach(s.match_id)"
          >说说话</button>
          <p v-else class="sent">已递出纸条，等对方回应</p>
        </article>
      </section>

      <section v-else class="none">
        <p>暂时没有契合的人。你的同频卡会一直替你留意。</p>
      </section>

      <!-- 收到的请求 -->
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
.meet {
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
.sub { margin: 4px 0 0; color: var(--mist); font-size: 14px; }
.error { color: #d98f6a; font-size: 14px; }
.hint { color: var(--mist); }

.join { margin-top: 28px; }
.join-lede {
  font-family: var(--serif);
  font-size: 16px;
  line-height: 1.8;
  max-width: 30ch;
  color: var(--paper);
}
.join-form { display: flex; flex-direction: column; gap: 14px; margin-top: 20px; }
.join-form label { display: flex; flex-direction: column; gap: 6px; }
.join-form span { color: var(--mist); font-size: 14px; }
select, .join-form input[type='checkbox'] { accent-color: var(--amber); }
select {
  font-family: inherit;
  background: var(--panel);
  color: var(--paper);
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 15px;
}
.age-row { display: flex; gap: 14px; }
.age-row label { flex: 1; }
.check { flex-direction: row !important; align-items: center; gap: 8px !important; }
.check span { color: var(--mist); font-size: 13.5px; }
.join-form button {
  margin-top: 8px;
  background: var(--amber);
  color: #191510;
  border-radius: 10px;
  padding: 11px 0;
  font-size: 15px;
  font-weight: 600;
}
.join-form button:disabled { opacity: 0.5; }

.warming { margin-top: 64px; text-align: center; }
.warm-num { font-family: var(--serif); font-size: 56px; margin: 0; }
.warm-text { color: var(--mist); font-size: 15px; line-height: 1.8; margin-top: 10px; }

.none { margin-top: 48px; text-align: center; color: var(--mist); font-size: 15px; }

h2 {
  font-size: 14.5px;
  color: var(--mist);
  font-weight: normal;
  margin: 26px 0 10px;
}
.sug, .req {
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--panel);
  padding: 16px;
  margin-bottom: 12px;
}
.hl {
  margin: 0 0 12px;
  padding: 0;
  list-style: none;
}
.hl li {
  font-family: var(--serif);
  font-size: 15.5px;
  padding: 4px 0;
  color: var(--paper);
}
.primary {
  background: var(--amber);
  color: #191510;
  border-radius: 10px;
  padding: 9px 18px;
  font-size: 14.5px;
  font-weight: 600;
  text-decoration: none;
  display: inline-block;
}
.primary:disabled { opacity: 0.5; }
.link { padding: 8px 16px; }
.ghost {
  background: none;
  border: 1px solid var(--line);
  color: var(--mist);
  border-radius: 10px;
  padding: 8px 16px;
  font-size: 14px;
}
.sent { color: var(--mist); font-size: 14px; margin: 0; }
.who { margin: 0 0 10px; font-family: var(--serif); font-size: 15.5px; }
.acts { display: flex; gap: 10px; }
</style>
