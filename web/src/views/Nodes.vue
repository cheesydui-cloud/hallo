<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'
import { copyText } from '../copy'

const items = ref([])
const error = ref('')
const hint = ref('')
const staged = ref({})
const busy = ref(false)
const editing = ref(null)
const form = ref({ name: '', public_host: '' })

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

async function create() {
  error.value = ''
  hint.value = ''
  busy.value = true
  try {
    const r = await api.createNode({
      name: form.value.name,
      public_host: form.value.public_host,
    })
    form.value = { name: '', public_host: '' }
    hint.value = r.install_hint || ''
    toastOk(r.message || '服务器已登记，请到那台机执行安装命令')
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
      enabled: n.enabled,
    })
    toastOk('已保存')
    editing.value = null
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function pushOne(id) {
  try {
    const r = await api.pushNodeUpdate(id)
    toastOk(r.message || '已标记推送，下次心跳会换包')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function pushAll() {
  if (!confirm('所有在线 Agent 下次心跳都会从本面板拉新包。继续？')) return
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
    toastErr('本机不能删除')
    return
  }
  if (!confirm('从面板删除这台服务器？机器上的 hallo-agent 不会自动卸载。')) return
  try {
    await api.deleteNode(n.id)
    toastOk('已删除')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

function seen(n) {
  if (n.is_local) return '面板本机，直接跑官方 Xray'
  if (!n.last_seen) return '从未上线：把安装命令拿到那台机用 root 执行'
  return new Date(n.last_seen).toLocaleString()
}

function xrayText(n) {
  if (n.is_local) return n.xray_running ? 'Xray 运行中' : n.xray_message || 'Xray 未启动'
  if (!n.online) return 'Agent 离线'
  return n.xray_running ? 'Xray 运行中' : n.xray_message || 'Xray 未启动'
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
          面板本机已经内嵌官方 Xray。其它机器登记后，把安装命令拿到那台机用 root 执行，等「在线」且 Xray 运行中。然后去「协议」选这台机添加节点并复制链接。
        </p>
      </div>
      <button class="btn-primary" @click="pushAll">一键推送全部 Agent</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>
    <p class="text-xs text-ink/40 mb-4">
      已暂存 agent：{{ Object.keys(staged).length ? Object.keys(staged).join(', ') : '无（先在设置里更新面板）' }}
    </p>

    <form class="card p-5 mb-6 grid md:grid-cols-3 gap-3 items-end" @submit.prevent="create">
      <div>
        <label class="label">名称</label>
        <input class="input" v-model="form.name" required placeholder="美国 / 香港" />
      </div>
      <div>
        <label class="label">公网 IP 或域名</label>
        <input class="input" v-model="form.public_host" placeholder="可空，Agent 上线后自动填" />
      </div>
      <button class="btn-primary h-10" type="submit" :disabled="busy">{{ busy ? '登记中…' : '添加服务器' }}</button>
    </form>

    <div v-if="hint" class="card p-5 mb-6">
      <div class="text-sm font-medium mb-1">到那台服务器用 root 执行（成功会打印「已安装并在 systemd 中运行」）</div>
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
            <p class="text-xs text-ink/45 mt-1">{{ n.public_host || n.host || '尚未上报地址' }} · {{ xrayText(n) }}</p>
            <p class="text-xs text-ink/35 mt-1">{{ seen(n) }}</p>
          </div>
          <div class="flex flex-wrap gap-1 justify-end">
            <button v-if="!n.is_local" class="btn-ghost text-xs" type="button" @click="copy(installCmd(n)); hint = installCmd(n)">复制安装命令</button>
            <button v-if="!n.is_local" class="btn-ghost text-xs" type="button" @click="pushOne(n.id)">推送更新</button>
            <button class="btn-ghost text-xs" type="button" @click="editing = editing === n.id ? null : n.id">{{ editing === n.id ? '收起' : '编辑' }}</button>
            <button v-if="!n.is_local" class="btn-ghost text-xs text-red-700" type="button" @click="remove(n)">删除</button>
          </div>
        </div>
        <form v-if="editing === n.id" class="grid md:grid-cols-3 gap-3 mt-4 pt-4 border-t border-ink/5" @submit.prevent="save(n)">
          <div>
            <label class="label">名称</label>
            <input class="input" v-model="n.name" />
          </div>
          <div>
            <label class="label">公网地址</label>
            <input class="input" v-model="n.public_host" :placeholder="n.host || '公网 IP'" />
          </div>
          <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" v-model="n.enabled" />
            启用
          </label>
          <button class="btn-primary" type="submit">保存</button>
        </form>
      </article>
      <div v-if="!items.length" class="card px-4 py-10 text-center text-ink/40">还没有服务器。初始化后会出现「本机」。</div>
    </div>
  </div>
</template>
