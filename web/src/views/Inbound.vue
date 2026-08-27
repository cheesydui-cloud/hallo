<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'
import { copyText } from '../copy'

const items = ref([])
const nodes = ref([])
const xray = ref({ running: false, path: '', message: '' })
const error = ref('')
const selected = ref(0)
const showForm = ref(false)
const busy = ref(false)
const editing = ref(null)

const empty = () => ({
  id: 0,
  node_id: 0,
  remark: '',
  tag: '',
  protocol: 'vless',
  listen: '0.0.0.0',
  port: 443,
  flow: 'xtls-rprx-vision',
  security: 'reality',
  dest: 'www.microsoft.com:443',
  server_name: 'www.microsoft.com',
  private_key: '',
  public_key: '',
  short_id: '',
  method: 'aes-128-gcm',
  password: '',
  enabled: true,
})
const form = ref(empty())

onMounted(load)
const poll = setInterval(load, 8000)
onUnmounted(() => clearInterval(poll))

async function load() {
  error.value = ''
  try {
    const [ib, n] = await Promise.all([api.inbounds(), api.nodes()])
    items.value = ib.items || []
    xray.value = { running: ib.xray_running, path: ib.xray_path, message: ib.xray_message }
    nodes.value = n.items || []
    if (!selected.value && nodes.value.length) {
      const local = nodes.value.find((x) => x.is_local) || nodes.value[0]
      selected.value = local.id
    }
    if (selected.value && !nodes.value.some((x) => x.id === selected.value) && nodes.value.length) {
      selected.value = nodes.value[0].id
    }
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

const current = computed(() => nodes.value.find((n) => n.id === selected.value) || null)
const filtered = computed(() => items.value.filter((i) => i.node_id === selected.value))

watch(selected, (id) => {
  if (id && form.value) form.value.node_id = id
})

function statusOf(n) {
  if (n.is_local) return n.xray_running ? 'Xray 运行中' : shortErr(n.xray_message) || 'Xray 未启动'
  if (!n.online) return 'Agent 离线'
  return n.xray_running ? 'Xray 运行中' : shortErr(n.xray_message) || 'Xray 未启动'
}

function shortErr(msg) {
  if (!msg) return ''
  if (msg.includes('address already in use') || msg.includes('failed to listen')) return '端口被占用'
  return msg.length > 48 ? msg.slice(0, 48) + '…' : msg
}

function bannerClass() {
  const n = current.value
  if (!n) return xray.value.running ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-amber-200 bg-amber-50 text-amber-900'
  if (n.is_local) return n.xray_running ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-amber-200 bg-amber-50 text-amber-900'
  if (!n.online) return 'border-amber-200 bg-amber-50 text-amber-900'
  return n.xray_running ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-amber-200 bg-amber-50 text-amber-900'
}

function bannerText() {
  const n = current.value
  if (!n) return xray.value.running ? '本机 Xray：运行中' : '本机 Xray：' + (xray.value.message || '未启动')
  if (n.is_local) return n.xray_running ? '本机 Xray：运行中' : '本机 Xray：' + (n.xray_message || xray.value.message || '未启动')
  if (!n.online) return '这台 Agent 离线，配置下发不了。先到「服务器」安装 Agent。'
  return n.xray_running ? '这台 Xray：运行中' : '这台 Xray：' + (n.xray_message || '未启动')
}

function usedPorts(nodeId, skipId) {
  return items.value
    .filter((i) => i.node_id === nodeId && i.enabled && i.id !== skipId)
    .map((i) => i.port)
}

function nextFreePort(nodeId) {
  const taken = new Set(usedPorts(nodeId, 0))
  const preferred = [443, 8443, 2053, 2083, 2087, 2096, 8080, 8880]
  for (const p of preferred) {
    if (!taken.has(p)) return p
  }
  for (let p = 10000; p < 20000; p++) {
    if (!taken.has(p)) return p
  }
  return 443
}

function applyProtocolDefaults() {
  const p = form.value.protocol
  if (p === 'vless') {
    form.value.security = 'reality'
    form.value.flow = form.value.flow || 'xtls-rprx-vision'
    form.value.dest = form.value.dest || 'www.microsoft.com:443'
    form.value.server_name = form.value.server_name || 'www.microsoft.com'
  } else if (p === 'vmess') {
    form.value.security = 'none'
    form.value.flow = ''
  } else if (p === 'shadowsocks') {
    form.value.security = 'none'
    form.value.flow = ''
    form.value.method = form.value.method || 'aes-128-gcm'
  }
}

function protocolLabel(row) {
  const p = (row.protocol || 'vless').toLowerCase()
  if (p === 'vmess') return 'VMess'
  if (p === 'shadowsocks' || p === 'ss') return 'Shadowsocks'
  if (row.security === 'none') return 'VLESS'
  return 'VLESS+Reality'
}

function protocolClass(row) {
  const p = (row.protocol || 'vless').toLowerCase()
  if (p === 'vmess') return 'bg-violet-50 text-violet-700'
  if (p === 'shadowsocks' || p === 'ss') return 'bg-amber-50 text-amber-800'
  return 'bg-sky-50 text-sky-700'
}

function extraOf(row) {
  const p = (row.protocol || 'vless').toLowerCase()
  if (p === 'shadowsocks' || p === 'ss') return row.method || 'aes-128-gcm'
  if (p === 'vmess') return 'tcp'
  return row.server_name || ''
}

function openAdd() {
  if (!current.value) {
    toastErr('先去「服务器」添加机器，远程机要先装 Agent')
    return
  }
  form.value = empty()
  form.value.node_id = current.value.id
  form.value.port = nextFreePort(current.value.id)
  form.value.remark = current.value.name
  form.value.tag = ''
  editing.value = null
  showForm.value = true
}

function openEdit(row) {
  form.value = { ...empty(), ...row }
  editing.value = row.id
  showForm.value = true
}

async function save() {
  if (!form.value.node_id) {
    toastErr('必须选择服务器')
    return
  }
  applyProtocolDefaults()
  busy.value = true
  try {
    const payload = {
      ...form.value,
      node_id: Number(form.value.node_id) || 0,
      port: Number(form.value.port) || 443,
    }
    const r = editing.value ? await api.updateInbound(editing.value, payload) : await api.createInbound(payload)
    if (r.warning) toastErr('已保存，但 Xray 未起来：' + r.warning)
    else toastOk(editing.value ? '已更新并下发到这台服务器' : '已添加协议并下发到这台服务器')
    showForm.value = false
    await load()
  } catch (e) {
    toastErr(e)
  } finally {
    busy.value = false
  }
}

async function toggle(row) {
  try {
    await api.updateInbound(row.id, { ...row, enabled: !row.enabled })
    toastOk(row.enabled ? '已停用' : '已启用')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function regen(row) {
  if (!confirm('重新生成该入站 Reality 密钥？客户端必须重新拉订阅。')) return
  try {
    const r = await api.regenInboundKeys(row.id)
    toastOk(r.warning ? '密钥已更新，但 Xray 未起来：' + r.warning : '密钥已更新并下发')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function remove(row) {
  if (!confirm('删除这条入站？')) return
  try {
    await api.deleteInbound(row.id)
    toastOk('已删除')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function reloadXray() {
  if (!current.value) {
    toastErr('先选一台服务器')
    return
  }
  try {
    const r = await api.reloadNodeXray(current.value.id)
    toastOk(r.message || (r.running ? 'Xray 已重载' : '已通知重启'))
    await load()
  } catch (e) {
    toastErr(e)
  }
}

function isReality(row) {
  const p = (row?.protocol || form.value.protocol || 'vless').toLowerCase()
  return p === 'vless' && (row?.security || form.value.security) !== 'none'
}

function copyShare(row) {
  if (!row.share_host || !row.share_link) {
    toastErr('这台服务器还没有公网 IP。到「服务器」填公网地址，或等 Agent 上线自动上报后再复制。')
    return
  }
  copyText(row.share_link, '已复制节点链接，可直接导入客户端')
}
</script>

<template>
  <div>
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-4">
      <div>
        <h2 class="text-xl font-semibold">协议</h2>
        <p class="text-sm text-black/45 mt-1">左边选服务器，右边在这台机器上添加 VLESS / VMess / Shadowsocks，点「复制」导入客户端。同一台机端口不能重复。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button class="btn-ghost" type="button" @click="reloadXray" :disabled="!current">重启这台 Xray</button>
        <button class="btn-primary" type="button" @click="openAdd" :disabled="!current">+ 添加协议</button>
      </div>
    </div>

    <div class="rounded-md border px-4 py-2.5 text-sm mb-4" :class="bannerClass()">
      <div>{{ bannerText() }}</div>
      <div v-if="current && current.is_local" class="font-mono text-xs opacity-70 mt-1">{{ xray.path }}</div>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>

    <div class="flex gap-4 items-start min-h-[420px]">
      <aside class="w-[240px] shrink-0 card overflow-hidden">
        <div class="px-3 py-2.5 text-xs text-black/45 border-b border-black/5">服务器</div>
        <button
          v-for="n in nodes"
          :key="n.id"
          type="button"
          class="w-full text-left px-3 py-3 border-b border-black/5 hover:bg-black/[0.03]"
          :class="selected === n.id ? 'bg-[#e6f4ff] border-l-2 border-l-[#1677ff]' : ''"
          @click="selected = n.id"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="font-medium text-sm truncate">{{ n.name }}</span>
            <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="n.online || (n.is_local && n.xray_running) ? 'bg-emerald-500' : 'bg-black/20'" />
          </div>
          <div class="text-[11px] text-black/40 mt-0.5 truncate">{{ n.public_host || n.host || '未上报 IP' }}</div>
          <div class="text-[11px] mt-0.5" :class="n.xray_running ? 'text-emerald-700' : 'text-amber-700'">{{ statusOf(n) }}</div>
        </button>
        <div v-if="!nodes.length" class="px-3 py-8 text-xs text-black/40 text-center">还没有服务器。先去「服务器」添加并安装 Agent。</div>
      </aside>

      <div class="flex-1 min-w-0">
        <div v-if="current" class="flex items-center justify-between mb-3">
          <div>
            <div class="font-medium">{{ current.name }} <span class="text-xs text-black/40 font-normal">{{ current.is_local ? '面板本机' : 'Agent' }}</span></div>
            <div class="text-xs text-black/45 mt-0.5">{{ current.public_host || current.host || '未上报 IP，复制链接前先填公网地址' }} · 在这台机器上添加不同协议</div>
          </div>
        </div>

        <div class="card table-wrap">
          <table class="ui min-w-[860px]">
            <thead>
              <tr>
                <th>启用</th>
                <th>备注</th>
                <th>端口</th>
                <th>协议</th>
                <th>客户端</th>
                <th>参数</th>
                <th>状态</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in filtered" :key="row.id">
                <td>
                  <input type="checkbox" :checked="row.enabled" @change="toggle(row)" />
                </td>
                <td>
                  <div class="font-medium">{{ row.remark || row.tag }}</div>
                  <div class="text-[11px] text-black/40 font-mono">{{ row.tag }} · {{ row.listen }}</div>
                </td>
                <td class="font-mono">{{ row.port }}</td>
                <td>
                  <span class="badge" :class="protocolClass(row)">{{ protocolLabel(row) }}</span>
                </td>
                <td>{{ row.client_num ?? 0 }}</td>
                <td class="text-xs font-mono">{{ extraOf(row) }}</td>
                <td>
                  <span class="badge" :class="row.keys_ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'">
                    {{ row.keys_ok ? '正常' : '无效' }}
                  </span>
                </td>
                <td class="text-right whitespace-nowrap space-x-1">
                  <button class="btn-primary text-xs" type="button" @click="copyShare(row)">复制</button>
                  <button class="btn-ghost text-xs" type="button" @click="openEdit(row)">编辑</button>
                  <button v-if="isReality(row)" class="btn-ghost text-xs" type="button" @click="regen(row)">换密钥</button>
                  <button class="btn-danger text-xs" type="button" @click="remove(row)">删除</button>
                </td>
              </tr>
              <tr v-if="current && !filtered.length">
                <td colspan="8" class="text-center text-black/40 py-10">
                  这台服务器还没有协议。点右上角「添加协议」。
                </td>
              </tr>
              <tr v-if="!current">
                <td colspan="8" class="text-center text-black/40 py-10">先在左边选一台服务器。</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div v-if="showForm" class="fixed inset-0 z-40 bg-black/40 flex items-start justify-center overflow-y-auto p-6" @click.self="showForm = false">
      <form class="bg-white rounded-lg shadow-xl w-full max-w-2xl p-6 space-y-4 my-8" @submit.prevent="save">
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{{ editing ? '编辑协议' : '添加协议' }}</h3>
          <button class="text-black/40" type="button" @click="showForm = false">×</button>
        </div>
        <div class="grid md:grid-cols-2 gap-3">
          <div class="md:col-span-2">
            <label class="label">跑在哪台服务器</label>
            <select class="input" v-model.number="form.node_id" required>
              <option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.name }} {{ n.is_local ? '· 本机' : '· Agent' }} · {{ n.public_host || n.host || '未上报 IP' }}</option>
            </select>
          </div>
          <div class="md:col-span-2">
            <label class="label">协议</label>
            <select class="input" v-model="form.protocol" @change="applyProtocolDefaults">
              <option value="vless">VLESS + Reality（推荐）</option>
              <option value="vmess">VMess TCP</option>
              <option value="shadowsocks">Shadowsocks</option>
            </select>
          </div>
          <div>
            <label class="label">备注</label>
            <input class="input" v-model="form.remark" placeholder="洛杉矶-443" />
          </div>
          <div>
            <label class="label">Tag</label>
            <input class="input font-mono text-xs" v-model="form.tag" placeholder="留空自动" />
          </div>
          <div>
            <label class="label">监听</label>
            <input class="input" v-model="form.listen" />
          </div>
          <div>
            <label class="label">端口</label>
            <input class="input" type="number" v-model.number="form.port" />
            <p class="text-[11px] text-black/40 mt-1">同一台服务器端口不能重复。443 被占用时换 8443 等。</p>
          </div>

          <template v-if="form.protocol === 'vless'">
            <div>
              <label class="label">Dest（回落）</label>
              <input class="input" v-model="form.dest" />
            </div>
            <div>
              <label class="label">SNI</label>
              <input class="input" v-model="form.server_name" />
            </div>
            <div>
              <label class="label">Flow</label>
              <select class="input" v-model="form.flow">
                <option value="xtls-rprx-vision">xtls-rprx-vision（推荐）</option>
                <option value="">无</option>
              </select>
            </div>
            <div>
              <label class="label">Short ID</label>
              <input class="input font-mono text-xs" v-model="form.short_id" placeholder="留空自动生成" />
            </div>
            <div class="md:col-span-2 text-xs text-black/40">Reality 密钥留空会自动生成。客户端用订阅，不必手填。</div>
          </template>

          <template v-if="form.protocol === 'shadowsocks'">
            <div>
              <label class="label">加密</label>
              <select class="input" v-model="form.method">
                <option value="aes-128-gcm">aes-128-gcm</option>
                <option value="aes-256-gcm">aes-256-gcm</option>
                <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
                <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm</option>
              </select>
            </div>
            <div>
              <label class="label">密码</label>
              <input class="input font-mono text-xs" v-model="form.password" placeholder="留空自动生成" />
            </div>
          </template>

          <p v-if="form.protocol === 'vmess'" class="md:col-span-2 text-xs text-black/40">VMess 用每个用户的 UUID，无需单独密码。明文 TCP，适合内网或已有 TLS 反代。</p>
        </div>
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" v-model="form.enabled" />
          启用
        </label>
        <div class="flex justify-end gap-2">
          <button class="btn-ghost" type="button" @click="showForm = false">取消</button>
          <button class="btn-primary" type="submit" :disabled="busy">{{ busy ? '保存中…' : '保存并下发' }}</button>
        </div>
      </form>
    </div>
  </div>
</template>
