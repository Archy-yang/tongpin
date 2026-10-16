<script setup>
import { onMounted, ref } from 'vue'
import {
  updateBasics, bindEmail, emailLogin, changePassword,
  requestResetCode, confirmReset, logout, getProactive, setProactive,
  getSubscription, startCheckout,
} from '../api.js'

const me = ref(null) // { id, nickname, email }
const proactive = ref(true) // AI 主动开话开关
const member = ref(null) // { active, plan, current_period_end }

// 基础设置
const nickname = ref('')
const city = ref('')
const gender = ref('')
const ageRange = ref('')
const genderPref = ref('')
const savingBasics = ref(false)
const savedBasics = ref(false)

// 账号区状态机：bind（未绑定）| login（登录已有账号）| reset（找回密码）| password（改密）
const panel = ref('bind')
const email = ref('')
const password = ref('')
const oldPassword = ref('')
const code = ref('')
const busy = ref(false)
const msg = ref('')      // 成功提示
const err = ref('')      // 错误提示

onMounted(async () => {
  try {
    const res = await fetch('/api/v1/me', {
      headers: { Authorization: `Bearer ${localStorage.getItem('tongpin_token')}` },
    })
    const json = await res.json()
    if (json.code === 0) {
      me.value = json.data
      nickname.value = json.data.nickname
      panel.value = json.data.email ? 'password' : 'bind'
    }
    const p = await getProactive()
    proactive.value = p.enabled
  } catch { /* 未登录也展示表单 */ }
  try {
    member.value = await getSubscription()
  } catch { /* 支付未接入时保持 null */ }
})

async function upgrade() {
  err.value = ''
  msg.value = ''
  try {
    const url = await startCheckout()
    location.href = url // 跳 Stripe 托管收银台
  } catch (e) {
    err.value = e.message // 如「支付暂未开放，敬请期待」
  }
}

async function toggleProactive() {
  proactive.value = !proactive.value
  try {
    await setProactive(proactive.value)
  } catch (e) {
    proactive.value = !proactive.value // 失败回滚
    err.value = e.message
  }
}

async function saveBasics() {
  savingBasics.value = true
  try {
    await updateBasics({
      nickname: nickname.value, city: city.value, gender: gender.value,
      age_range: ageRange.value, gender_pref: genderPref.value,
    })
    savedBasics.value = true
    setTimeout(() => (savedBasics.value = false), 2000)
  } catch (e) {
    err.value = e.message
  } finally {
    savingBasics.value = false
  }
}

async function run(fn) {
  busy.value = true
  err.value = ''
  msg.value = ''
  try {
    await fn()
  } catch (e) {
    err.value = e.message
  } finally {
    busy.value = false
  }
}

const doBind = () => run(async () => {
  await bindEmail(email.value, password.value)
  msg.value = '绑定成功，以后用邮箱就能登录了'
  await reload()
})

const doLogin = () => run(async () => {
  await emailLogin(email.value, password.value)
  location.reload() // 切换账号，整页刷新最干净
})

const doSendCode = () => run(async () => {
  await requestResetCode(email.value)
  msg.value = '验证码已发到邮箱（开发模式下看后端日志）'
})

const doReset = () => run(async () => {
  await confirmReset(email.value, code.value, password.value)
  msg.value = '密码已重置，用新密码登录吧'
  panel.value = 'login'
})

const doChangePassword = () => run(async () => {
  await changePassword(oldPassword.value, password.value)
  msg.value = '密码已修改'
  oldPassword.value = password.value = ''
})

function doLogout() {
  logout()
  location.href = '/' // 回到匿名新账号
}

function fmtDate(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })
}

async function reload() {
  const res = await fetch('/api/v1/me', {
    headers: { Authorization: `Bearer ${localStorage.getItem('tongpin_token')}` },
  })
  const json = await res.json()
  if (json.code === 0) me.value = json.data
  if (me.value.email) panel.value = 'password'
}
</script>

<template>
  <main class="me-page">
    <header class="head"><h1>我的</h1></header>

    <!-- 账号 -->
    <section class="card">
      <h2>账号</h2>
      <p v-if="me?.email" class="account-line">
        {{ me.email }}
        <span class="tag">邮箱登录</span>
      </p>
      <p v-else-if="me" class="account-line">
        匿名账号 · {{ me.nickname }}
        <span class="tag ghost-tag">未绑定</span>
      </p>

      <!-- 绑定邮箱 -->
      <form v-if="panel === 'bind'" class="stack" @submit.prevent="doBind">
        <p class="hint">绑定邮箱后，换设备也能登录，忘记密码可找回。聊天记录和同频卡都会保留。</p>
        <input v-model="email" type="email" placeholder="邮箱" required />
        <input v-model="password" type="password" placeholder="设置密码（至少 8 位）" required minlength="8" />
        <button class="primary" :disabled="busy">{{ busy ? '…' : '绑定邮箱' }}</button>
        <button type="button" class="linklike" @click="panel = 'login'">已有账号？去登录</button>
      </form>

      <!-- 登录已有账号 -->
      <form v-else-if="panel === 'login'" class="stack" @submit.prevent="doLogin">
        <p class="hint">登录后将以该账号的身份继续（当前匿名数据不再显示）。</p>
        <input v-model="email" type="email" placeholder="邮箱" required />
        <input v-model="password" type="password" placeholder="密码" required />
        <button class="primary" :disabled="busy">{{ busy ? '…' : '登录' }}</button>
        <div class="row-between">
          <button type="button" class="linklike" @click="panel = 'reset'">忘记密码？</button>
          <button type="button" class="linklike" v-if="!me?.email" @click="panel = 'bind'">绑定新邮箱</button>
        </div>
      </form>

      <!-- 找回密码 -->
      <form v-else-if="panel === 'reset'" class="stack" @submit.prevent="doReset">
        <p class="hint">输入注册邮箱，我们会发一个 6 位验证码。</p>
        <input v-model="email" type="email" placeholder="邮箱" required />
        <div class="code-row">
          <input v-model="code" placeholder="验证码" maxlength="6" required />
          <button type="button" class="ghost" :disabled="busy" @click="doSendCode">发验证码</button>
        </div>
        <input v-model="password" type="password" placeholder="新密码（至少 8 位）" required minlength="8" />
        <button class="primary" :disabled="busy">{{ busy ? '…' : '重置密码' }}</button>
        <button type="button" class="linklike" @click="panel = 'login'">返回登录</button>
      </form>

      <!-- 已绑定：改密 / 退出 -->
      <form v-else class="stack" @submit.prevent="doChangePassword">
        <input v-model="oldPassword" type="password" placeholder="当前密码" required />
        <input v-model="password" type="password" placeholder="新密码（至少 8 位）" required minlength="8" />
        <button class="primary" :disabled="busy">{{ busy ? '…' : '修改密码' }}</button>
        <button type="button" class="ghost danger" @click="doLogout">退出登录</button>
      </form>

      <p v-if="msg" class="ok" role="status">{{ msg }}</p>
      <p v-if="err" class="error" role="alert">{{ err }}</p>
    </section>

    <!-- 会员 -->
    <section class="card">
      <h2>会员</h2>
      <p v-if="member?.active" class="account-line">
        同频会员 · 至 {{ fmtDate(member.current_period_end) }}
        <span class="tag">生效中</span>
      </p>
      <template v-else>
        <p class="hint">会员畅聊不限量：AI 聊天额度大幅提升、记忆更深、主动开话。</p>
        <button class="primary" @click="upgrade">升级会员 · ¥15/月</button>
      </template>
    </section>

    <!-- 基础设置 -->
    <section class="card">
      <h2>设置</h2>
      <form class="stack" @submit.prevent="saveBasics">
        <input v-model="nickname" maxlength="32" placeholder="昵称" />
        <input v-model="city" maxlength="32" placeholder="城市，如：北京" />
        <div class="row-between">
          <select v-model="gender">
            <option value="">性别不填</option>
            <option value="male">男</option>
            <option value="female">女</option>
            <option value="other">其他</option>
          </select>
          <select v-model="ageRange">
            <option value="">年龄不填</option>
            <option value="18-24">18-24</option>
            <option value="25-30">25-30</option>
            <option value="31-40">31-40</option>
            <option value="40+">40 以上</option>
          </select>
        </div>
        <select v-model="genderPref">
          <option value="">想认识：都可以</option>
          <option value="male">想认识：男生</option>
          <option value="female">想认识：女生</option>
          <option value="other">想认识：不限性别</option>
        </select>
        <button class="primary" :disabled="savingBasics">{{ savingBasics ? '保存中…' : '保存' }}</button>
        <p v-if="savedBasics" class="ok" role="status">已保存</p>
      </form>
      <label class="proactive">
        <input type="checkbox" :checked="proactive" @change="toggleProactive" />
        <span>允许 AI 在你的活跃时段主动找你（每天最多一次，可随时关闭）</span>
      </label>
      <p class="privacy">这里填的信息只用于匹配筛选。你不需要头像，也不需要真实姓名。</p>
    </section>
  </main>
</template>

<style scoped>
.me-page { flex: 1; overflow-y: auto; padding: 28px 22px 30px; }
.head h1 { font-family: var(--serif); font-weight: 600; font-size: 26px; margin: 0 0 20px; }
.card {
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--panel);
  padding: 18px 16px;
  margin-bottom: 16px;
}
.card h2 { font-size: 14.5px; color: var(--mist); font-weight: normal; margin: 0 0 12px; }
.account-line { margin: 0 0 12px; font-family: var(--serif); font-size: 15.5px; display: flex; align-items: center; gap: 8px; }
.tag { font-size: 12px; color: var(--amber); border: 1px solid var(--amber); border-radius: 6px; padding: 1px 8px; }
.ghost-tag { color: var(--mist); border-color: var(--line); }
.stack { display: flex; flex-direction: column; gap: 10px; }
.stack input, .stack select {
  background: var(--ink); border: 1px solid var(--line); border-radius: 10px;
  padding: 10px 12px; color: var(--paper); font-size: 15px; font-family: inherit;
  width: 100%; box-sizing: border-box;
}
.stack input::placeholder { color: var(--mist); }
.primary { background: var(--amber); color: #191510; border-radius: 10px; padding: 11px 0; font-size: 15px; font-weight: 600; }
.primary:disabled { opacity: 0.5; }
.ghost { background: none; border: 1px solid var(--line); color: var(--mist); border-radius: 10px; padding: 10px 0; font-size: 14px; width: 100%; }
.danger:hover { border-color: #d98f6a; color: #d98f6a; }
.linklike { background: none; border: none; color: var(--mist); font-size: 13.5px; padding: 2px 0; text-align: left; }
.linklike:hover { color: var(--amber); }
.row-between { display: flex; gap: 10px; justify-content: space-between; align-items: center; }
.row-between select { flex: 1; background: var(--ink); border: 1px solid var(--line); border-radius: 10px; padding: 10px 12px; color: var(--paper); font-size: 15px; }
.code-row { display: flex; gap: 10px; }
.code-row input { flex: 1; letter-spacing: 0.3em; }
.code-row .ghost { width: auto; padding: 10px 14px; flex: none; }
.hint { margin: 0; color: var(--mist); font-size: 13.5px; line-height: 1.7; }
.ok { color: var(--amber); font-size: 14px; margin: 10px 0 0; }
.error { color: #d98f6a; font-size: 14px; margin: 10px 0 0; }
.privacy { margin: 14px 0 0; color: var(--mist); font-size: 13px; line-height: 1.7; }
.proactive {
  display: flex; align-items: flex-start; gap: 8px;
  margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--line);
  cursor: pointer;
}
.proactive input { accent-color: var(--amber); margin-top: 3px; }
.proactive span { color: var(--mist); font-size: 13.5px; line-height: 1.6; }
</style>
