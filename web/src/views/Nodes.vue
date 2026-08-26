<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'

const items = ref([])
const name = ref('')
const error = ref('')
const hint = ref('')
const staged = ref({})
const busy = ref(false)

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
    const r = await api.createNode({ name: name.value })
    name.value = ''
    hint.value = r.install_hint || ''
    toastOk(r.message || '节点已登记，请把命令拿到节点机用 root 执行')
    await load()
  } catch (e) {
    error.value = e.message
    toastErr(e)
  } finally {
    busy.value = false
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

async function remove(id) {
  if (!confirm('删除该节点？节点机上的 hallo-agent 服务不会自动卸载。')) return
  try {
    await api.deleteNode(id)
    toastOk('节点已从面板删除')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

function seen(n) {
  if (!n.last_seen) return '从未上线（命令还没在节点机跑成功）'
  return new Date(n.last_seen).toLocaleString()
}

async function copy(text) {
  try {
    await navigator.clipboard.writeText(text)
    toastOk('安装命令已复制')
  } catch {
    toastErr('复制失败，请手动选中命令')
  }
}
</script>

<template>
  <div>
    <div class="flex items-end justify-between mb-6">
      <div>
        <h2 class="font-display text-3xl">节点 / Agent</h2>
        <p class="text-ink/50 text-sm mt-1">
          添加节点只是在面板登记。必须把下面的命令拿到<strong>节点机 root</strong> 执行，装上 systemd 后才会真正在线。
        </p>
      </div>
      <button class="btn-primary" @click="pushAll">一键推送全部 Agent</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>
    <p class="text-xs text-ink/40 mb-4">
      已暂存 agent：{{ Object.keys(staged).length ? Object.keys(staged).join(', ') : '无（先在设置里更新面板）' }}
    </p>

    <form class="card p-5 mb-6 flex gap-3 items-end" @submit.prevent="create">
      <div class="flex-1">
        <label class="label">节点名</label>
        <input class="input" v-model="name" required placeholder="hk-1" />
      </div>
      <button class="btn-primary h-10" type="submit" :disabled="busy">{{ busy ? '登记中…' : '添加节点' }}</button>
    </form>

    <div v-if="hint" class="card p-5 mb-6">
      <div class="text-sm font-medium mb-1">节点机用 root 执行（成功会打印「已安装并在 systemd 中运行」）</div>
      <pre class="text-xs whitespace-pre-wrap break-all bg-ink/[0.04] rounded-xl p-3">{{ hint }}</pre>
      <button class="btn-ghost text-xs mt-3" @click="copy(hint)">复制命令</button>
    </div>

    <div class="card overflow-hidden">
      <table class="w-full text-sm">
        <thead class="bg-ink/[0.03] text-ink/50 text-left">
          <tr>
            <th class="px-4 py-3 font-medium">节点</th>
            <th class="px-4 py-3 font-medium">版本</th>
            <th class="px-4 py-3 font-medium">状态</th>
            <th class="px-4 py-3 font-medium"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in items" :key="n.id" class="border-t border-ink/5">
            <td class="px-4 py-3">
              <div class="font-medium">{{ n.name }}</div>
              <div class="text-xs text-ink/40">{{ n.host || '—' }} · {{ n.arch || '未知架构' }}</div>
              <div class="text-[11px] text-ink/35 break-all mt-1">token {{ n.token }}</div>
            </td>
            <td class="px-4 py-3">
              {{ n.version || '—' }}
              <div v-if="n.force_update" class="text-xs text-accent">待更新 → {{ n.desired_version }}</div>
            </td>
            <td class="px-4 py-3">
              <span :class="n.online ? 'text-pine' : 'text-ink/40'">{{ n.online ? '在线' : '离线' }}</span>
              <div class="text-xs text-ink/35">{{ seen(n) }}</div>
            </td>
            <td class="px-4 py-3 text-right whitespace-nowrap space-x-1">
              <button class="btn-ghost text-xs" @click="copy(`curl -fsSL '${location.origin}/install/agent.sh?token=${n.token}' | sh`)">复制安装命令</button>
              <button class="btn-ghost text-xs" @click="pushOne(n.id)">推送更新</button>
              <button class="btn-ghost text-xs text-red-700" @click="remove(n.id)">删除</button>
            </td>
          </tr>
          <tr v-if="!items.length">
            <td colspan="4" class="px-4 py-10 text-center text-ink/40">还没有节点。单机只用面板时可以不填。</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
