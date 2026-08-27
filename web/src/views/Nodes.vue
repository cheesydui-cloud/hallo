<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'
import { copyText } from '../copy'

const items = ref([])
const error = ref('')
const hint = ref('')
const staged = ref({})
const busy = ref(false)
const editing = ref(null)
const form = ref({ name: '', public_host: '', port: 443, relay_node_id: 0 })

onMounted(load)

async function load() {
  error.value = ''
  try {
    const r = await api.nodes()
    items.value = r.items || []
    staged.value = r.agent_staged || {}
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

const relayOptions = computed(() => items.value.filter((n) => n.enabled))

async function create() {
  error.value = ''
  hint.value = ''
  busy.value = true
  try {
    const payload = {
      name: form.value.name,
      public_host: form.value.public_host,
      port: Number(form.value.port) || 443,
    }
    if (form.value.relay_node_id) payload.relay_node_id = Number(form.value.relay_node_id)
    const r = await api.createNode(payload)
    form.value = { name: '', public_host: '', port: 443, relay_node_id: 0 }
    hint.value = r.install_hint || ''
    toastOk(r.message || '节点已登记，请到节点机执行安装命令')
    await load()
  } catch (e) {
    error.value = e.message
    toastErr(e)
  } finally {
    busy.value = false
  }
}

async function save(n) {
  try {
    await api.updateNode(n.id, {
      name: n.name,
      public_host: n.public_host,
      port: Number(n.port) || 443,
      relay_node_id: n.relay_node_id || 0,
      subscribe: n.subscribe,
      enabled: n.enabled,
    })
    toastOk('节点已保存，配置会在下次心跳下发')
    editing.value = null
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function pushOne(id) {
  try {
    const r = await api.pushNodeUpdate(id)
    toastOk(r.message || '已标记推送，节点下次心跳会换包')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function pushAll() {
  if (!confirm('所有在线节点下次心跳都会从本面板拉新 agent。继续？')) return
  try {
    const r = await api.pushAllNodeUpdates()
    toastOk('已向全部节点标记更新 ' + (r.desired_version || ''))
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function remove(n) {
  if (n.is_local) {
    toastErr('本机节点不能删除')
    return
  }
  if (!confirm('删除该节点？节点机上的 hallo-agent 不会自动卸载。')) return
  try {
    await api.deleteNode(n.id)
    toastOk('节点已从面板删除')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

function seen(n) {
  if (n.is_local) return '面板本机，直接跑 Xray'
  if (!n.last_seen) return '从未上线（命令还没在节点机跑成功）'
  return new Date(n.last_seen).toLocaleString()
}

function relayName(n) {
  if (!n.relay_node_id) return ''
  const t = items.value.find((x) => x.id === n.relay_node_id)
  return t ? t.name : '#' + n.relay_node_id
}

	async function copy(text) {
	  await copyText(text)
	}

function installCmd(n) {
  return `curl -fsSL '${location.origin}/install/agent.sh?token=${n.token}' | sh`
}
</script>

<template>
  <div>
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-4 mb-6">
      <div>
        <h2 class="text-xl font-semibold">服务器</h2>
        <p class="text-black/45 text-sm mt-1 max-w-2xl">
          先把机器登记进来，复制安装命令到那台机用 root 执行。Agent 会装官方 Xray 并心跳。
          然后再去「入站」，选这台服务器添加 VLESS。订阅用的是<strong>这台机的公网 IP + 入站端口</strong>，不是面板 18080。
        </p>
      </div>
      <button class="btn-primary" @click="pushAll">一键推送全部 Agent</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>
    <p class="text-xs text-ink/40 mb-4">
      已暂存 agent：{{ Object.keys(staged).length ? Object.keys(staged).join(', ') : '无（先在设置里更新面板，或重跑安装脚本）' }}
    </p>

    <form class="card p-5 mb-6 grid md:grid-cols-5 gap-3 items-end" @submit.prevent="create">
      <div>
        <label class="label">节点名</label>
        <input class="input" v-model="form.name" required placeholder="洛杉矶 / 香港" />
      </div>
      <div>
        <label class="label">公网地址（IP 或域名）</label>
        <input class="input" v-model="form.public_host" placeholder="可空，上线后自动填" />
      </div>
      <div>
        <label class="label">入站端口</label>
        <input class="input" type="number" v-model.number="form.port" />
      </div>
      <div>
        <label class="label">链式转发到</label>
        <select class="input" v-model.number="form.relay_node_id">
          <option :value="0">直连出网</option>
          <option v-for="n in relayOptions" :key="n.id" :value="n.id">{{ n.name }}</option>
        </select>
      </div>
      <button class="btn-primary h-10" type="submit" :disabled="busy">{{ busy ? '登记中…' : '添加节点' }}</button>
    </form>

    <div v-if="hint" class="card p-5 mb-6">
      <div class="text-sm font-medium mb-1">到节点机用 root 执行（成功会打印「已安装并在 systemd 中运行」）</div>
      <pre class="text-xs whitespace-pre-wrap break-all bg-ink/[0.04] rounded-xl p-3">{{ hint }}</pre>
      <button class="btn-ghost text-xs mt-3" type="button" @click="copy(hint)">复制命令</button>
    </div>

    <div class="space-y-3">
      <article v-for="n in items" :key="n.id" class="card p-5">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="flex items-center gap-2">
              <h3 class="font-medium text-lg">{{ n.name }}</h3>
              <span class="text-[11px] uppercase tracking-wide px-2 py-0.5 rounded-full" :class="n.is_local ? 'bg-pine/10 text-pine' : 'bg-ink/5 text-ink/50'">
                {{ n.is_local ? '本机' : 'Agent' }}
              </span>
              <span class="text-[11px]" :class="n.online || n.is_local ? 'text-pine' : 'text-ink/40'">
                {{ n.online || n.is_local ? '在线' : '离线' }}
              </span>
            </div>
            <p class="text-xs text-ink/45 mt-1">
              {{ n.public_host || n.host || '尚未上报地址' }}:{{ n.port || 443 }}
              · Xray {{ n.xray_running || (n.is_local && n.online) ? '运行中' : (n.xray_message || '未启动') }}
            </p>
            <p class="text-xs text-ink/35 mt-1">{{ seen(n) }}</p>
            <p v-if="n.relay_node_id" class="text-xs text-accent mt-1">链式转发 → {{ relayName(n) }}</p>
          </div>
          <div class="flex flex-wrap gap-1 justify-end">
            <button v-if="!n.is_local" class="btn-ghost text-xs" type="button" @click="copy(installCmd(n)); hint = installCmd(n)">复制安装命令</button>
            <button v-if="!n.is_local" class="btn-ghost text-xs" type="button" @click="pushOne(n.id)">推送更新</button>
            <button class="btn-ghost text-xs" type="button" @click="editing = editing === n.id ? null : n.id">{{ editing === n.id ? '收起' : '编辑' }}</button>
            <button v-if="!n.is_local" class="btn-ghost text-xs text-red-700" type="button" @click="remove(n)">删除</button>
          </div>
        </div>
        <form v-if="editing === n.id" class="grid md:grid-cols-4 gap-3 mt-4 pt-4 border-t border-ink/5" @submit.prevent="save(n)">
          <div>
            <label class="label">名称</label>
            <input class="input" v-model="n.name" />
          </div>
          <div>
            <label class="label">公网地址</label>
            <input class="input" v-model="n.public_host" :placeholder="n.host || '节点公网 IP'" />
          </div>
          <div>
            <label class="label">入站端口</label>
            <input class="input" type="number" v-model.number="n.port" />
          </div>
          <div>
            <label class="label">链式转发到</label>
            <select class="input" v-model.number="n.relay_node_id">
              <option :value="0">直连出网</option>
              <option v-for="o in items.filter((x) => x.id !== n.id)" :key="o.id" :value="o.id">{{ o.name }}</option>
            </select>
          </div>
          <label class="flex items-center gap-2 text-sm md:col-span-2">
            <input type="checkbox" v-model="n.subscribe" />
            写入订阅
          </label>
          <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" v-model="n.enabled" />
            启用
          </label>
          <button class="btn-primary" type="submit">保存</button>
        </form>
      </article>
      <div v-if="!items.length" class="card px-4 py-10 text-center text-ink/40">还没有节点。初始化后会出现「本机」。</div>
    </div>
  </div>
</template>
