async function request(path, opts = {}) {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
    ...opts,
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  })
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { error: text }
  }
  if (!res.ok) {
    throw new Error(data?.error || res.statusText)
  }
  return data
}

export const api = {
  meta: () => request('/api/meta'),
  setup: (body) => request('/api/setup', { method: 'POST', body }),
  login: (body) => request('/api/login', { method: 'POST', body }),
  logout: () => request('/api/logout', { method: 'POST', body: {} }),
  dashboard: () => request('/api/dashboard'),
  plans: () => request('/api/plans'),
  createPlan: (body) => request('/api/plans', { method: 'POST', body }),
  updatePlan: (id, body) => request(`/api/plans/${id}`, { method: 'PUT', body }),
  deletePlan: (id) => request(`/api/plans/${id}`, { method: 'DELETE' }),
  users: () => request('/api/users'),
  createUser: (body) => request('/api/users', { method: 'POST', body }),
  updateUser: (id, body) => request(`/api/users/${id}`, { method: 'PUT', body }),
  deleteUser: (id) => request(`/api/users/${id}`, { method: 'DELETE' }),
  resetTraffic: (id) => request(`/api/users/${id}/reset-traffic`, { method: 'POST', body: {} }),
  resetUUID: (id) => request(`/api/users/${id}/reset-uuid`, { method: 'POST', body: {} }),
  inbound: () => request('/api/inbound'),
  saveInbound: (body) => request('/api/inbound', { method: 'PUT', body }),
  regenKeys: () => request('/api/inbound/regen-keys', { method: 'POST', body: {} }),
  inbounds: () => request('/api/inbounds'),
  createInbound: (body) => request('/api/inbounds', { method: 'POST', body }),
  updateInbound: (id, body) => request(`/api/inbounds/${id}`, { method: 'PUT', body }),
  deleteInbound: (id) => request(`/api/inbounds/${id}`, { method: 'DELETE' }),
  regenInboundKeys: (id) => request(`/api/inbounds/${id}/regen-keys`, { method: 'POST', body: {} }),
  outbounds: () => request('/api/outbounds'),
  createOutbound: (body) => request('/api/outbounds', { method: 'POST', body }),
  updateOutbound: (id, body) => request(`/api/outbounds/${id}`, { method: 'PUT', body }),
  deleteOutbound: (id) => request(`/api/outbounds/${id}`, { method: 'DELETE' }),
  reloadXray: () => request('/api/xray/reload', { method: 'POST', body: {} }),
  settings: () => request('/api/settings'),
  saveSettings: (body) => request('/api/settings', { method: 'PUT', body }),
  updateStatus: () => request('/api/update'),
  applyUpdate: () => request('/api/update', { method: 'POST', body: {} }),
  nodes: () => request('/api/nodes'),
  createNode: (body) => request('/api/nodes', { method: 'POST', body }),
  updateNode: (id, body) => request(`/api/nodes/${id}`, { method: 'PUT', body }),
  deleteNode: (id) => request(`/api/nodes/${id}`, { method: 'DELETE' }),
  pushNodeUpdate: (id) => request(`/api/nodes/${id}/push-update`, { method: 'POST', body: {} }),
  pushAllNodeUpdates: () => request('/api/nodes/push-update', { method: 'POST', body: {} }),
}

export function formatBytes(n) {
  const v = Number(n) || 0
  if (v === 0) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let x = v
  while (x >= 1024 && i < u.length - 1) {
    x /= 1024
    i++
  }
  return `${x.toFixed(i === 0 ? 0 : 1)} ${u[i]}`
}
