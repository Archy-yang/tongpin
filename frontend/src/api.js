// 极简 API 层：fetch + 统一响应解包 + 自动登录态。

const TOKEN_KEY = 'tongpin_token'
const DEVICE_KEY = 'tongpin_device'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY)
}

// ensureLogin 设备匿名登录：无 token 时自动注册（产品定位是无门槛进入）。
export async function ensureLogin() {
  const existing = getToken()
  if (existing) return me(existing).catch(() => loginFresh())
  return loginFresh()
}

async function loginFresh() {
  let deviceId = localStorage.getItem(DEVICE_KEY)
  if (!deviceId) {
    deviceId = crypto.randomUUID()
    localStorage.setItem(DEVICE_KEY, deviceId)
  }
  const data = await request('POST', '/auth/device', { device_id: deviceId })
  localStorage.setItem(TOKEN_KEY, data.token)
  return data.user
}

export async function updateBasics(fields) {
  return request('PUT', '/me', fields)
}

export async function loadChatHistory(before) {
  const q = before ? `?before=${before}&limit=30` : '?limit=30'
  const data = await request('GET', `/chat/messages${q}`)
  return data.messages ?? []
}

export async function sendChatMessage(content) {
  const data = await request('POST', '/chat/messages', { content })
  return data // { user_message, ai_message }
}

export async function loadStyleProfile() {
  return request('GET', '/style-profile')
}

export async function deleteStyleProfile() {
  return request('DELETE', '/style-profile')
}

async function me(token) {
  const data = await request('GET', '/me', null, token)
  return data
}

async function request(method, path, body, token = getToken()) {
  const headers = { 'Content-Type': 'application/json' }
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  const json = await res.json().catch(() => ({ code: -1, msg: '响应解析失败' }))
  if (json.code !== 0) {
    const err = new Error(json.msg || '请求失败')
    err.code = json.code
    throw err
  }
  return json.data
}
