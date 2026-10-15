<script setup>
import { onMounted, ref } from 'vue'
import { updateBasics } from '../api.js'

const nickname = ref('')
const city = ref('')
const gender = ref('')
const ageRange = ref('')
const genderPref = ref('')
const saving = ref(false)
const saved = ref(false)
const error = ref('')

onMounted(async () => {
  try {
    const token = localStorage.getItem('tongpin_token')
    const res = await fetch('/api/v1/me', {
      headers: { Authorization: `Bearer ${token}` },
    })
    const json = await res.json()
    if (json.code === 0) {
      nickname.value = json.data.nickname
    }
  } catch { /* 显示默认即可 */ }
})

async function save() {
  saving.value = true
  saved.value = false
  error.value = ''
  try {
    await updateBasics({
      nickname: nickname.value,
      city: city.value,
      gender: gender.value,
      age_range: ageRange.value,
      gender_pref: genderPref.value,
    })
    saved.value = true
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <main class="me-page">
    <header class="head">
      <h1>我的</h1>
    </header>

    <form class="form" @submit.prevent="save">
      <label>
        <span>昵称</span>
        <input v-model="nickname" maxlength="32" placeholder="匿名昵称" />
      </label>
      <label>
        <span>城市</span>
        <input v-model="city" maxlength="32" placeholder="如：北京" />
      </label>
      <label>
        <span>性别</span>
        <select v-model="gender">
          <option value="">不填写</option>
          <option value="male">男</option>
          <option value="female">女</option>
          <option value="other">其他</option>
        </select>
      </label>
      <label>
        <span>年龄段</span>
        <select v-model="ageRange">
          <option value="">不填写</option>
          <option value="18-24">18-24</option>
          <option value="25-30">25-30</option>
          <option value="31-40">31-40</option>
          <option value="40+">40 以上</option>
        </select>
      </label>
      <label>
        <span>想认识</span>
        <select v-model="genderPref">
          <option value="">都可以</option>
          <option value="male">男生</option>
          <option value="female">女生</option>
          <option value="other">不限性别</option>
        </select>
      </label>

      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="saved" class="ok">已保存</p>

      <button type="submit" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</button>
    </form>

    <p class="privacy">
      这里填的信息只用于匹配筛选。
      你不需要头像，也不需要真实姓名。
    </p>
  </main>
</template>

<style scoped>
.me-page {
  flex: 1;
  overflow-y: auto;
  padding: 28px 22px 30px;
}
.head h1 {
  font-family: var(--serif);
  font-weight: 600;
  font-size: 26px;
  margin: 0 0 20px;
}
.form { display: flex; flex-direction: column; gap: 14px; }
label { display: flex; flex-direction: column; gap: 6px; }
label span { color: var(--mist); font-size: 14px; }
select {
  font-family: inherit;
  background: var(--panel);
  color: var(--paper);
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 15px;
}
.form button {
  margin-top: 8px;
  background: var(--amber);
  color: #191510;
  border-radius: 10px;
  padding: 11px 0;
  font-size: 15px;
  font-weight: 600;
}
.form button:disabled { opacity: 0.5; }
.error { color: #d98f6a; font-size: 14px; margin: 0; }
.ok { color: var(--amber); font-size: 14px; margin: 0; }
.privacy {
  margin-top: 28px;
  color: var(--mist);
  font-size: 13px;
  line-height: 1.7;
}
</style>
