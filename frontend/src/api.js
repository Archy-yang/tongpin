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

// ---- 邮箱账号体系 ----

export async function emailRegister(email, password) {
  const data = await request('POST', '/auth/email/register', { email, password })
  localStorage.setItem(TOKEN_KEY, data.token)
  return data.user
}

export async function emailLogin(email, password) {
  const data = await request('POST', '/auth/email/login', { email, password })
  localStorage.setItem(TOKEN_KEY, data.token)
  return data.user
}

export async function bindEmail(email, password) {
  return request('POST', '/auth/email/bind', { email, password })
}

export async function changePassword(oldPassword, newPassword) {
  return request('POST', '/auth/password', { old_password: oldPassword, new_password: newPassword })
}

export async function requestResetCode(email) {
  return request('POST', '/auth/email/reset-request', { email })
}

export async function confirmReset(email, code, newPassword) {
  return request('POST', '/auth/email/reset-confirm', { email, code, new_password: newPassword })
}

export function logout() {
  localStorage.removeItem(TOKEN_KEY)
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

// ---- 匹配与真人接触 ----

export async function getOptIn() {
  return request('GET', '/match/opt-in') // 从未 opt-in 时 data 为 null
}

export async function optIn(payload) {
  return request('POST', '/match/opt-in', payload)
}

export async function withdrawOptIn() {
  return request('DELETE', '/match/opt-in')
}

export async function loadSuggestions() {
  return request('GET', '/match/suggestions')
}

export async function createContactRequest(matchId) {
  return request('POST', '/contact/requests', { match_id: matchId })
}

export async function listRequests(box) {
  const data = await request('GET', `/contact/requests?box=${box}`)
  return data.requests ?? []
}

export async function approveRequest(id) {
  return request('POST', `/contact/requests/${id}/approve`)
}

export async function rejectRequest(id) {
  return request('POST', `/contact/requests/${id}/reject`)
}

export async function revokeRequest(id) {
  return request('POST', `/contact/requests/${id}/revoke`)
}

export async function loadDirect(matchId, before) {
  const q = before ? `?before=${before}&limit=30` : '?limit=30'
  const data = await request('GET', `/dm/${matchId}/messages${q}`)
  return data.messages ?? []
}

export async function sendDirect(matchId, content) {
  return request('POST', `/dm/${matchId}/messages`, { content })
}

// ---- AI 暖场与聊后反馈（P1）----

export async function createIcebreak(matchId) {
  return request('POST', '/icebreak/requests', { match_id: matchId })
}

export async function myIcebreaks() {
  const data = await request('GET', '/icebreak/mine')
  return data.sessions ?? []
}

export async function approveIcebreak(id) {
  return request('POST', `/icebreak/sessions/${id}/approve`)
}

export async function declineIcebreak(id) {
  return request('POST', `/icebreak/sessions/${id}/decline`)
}

export async function sendFeedback(matchId, direction) {
  return request('POST', '/match/feedback', { match_id: matchId, direction })
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
