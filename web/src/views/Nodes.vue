<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'

const items = ref([])
const name = ref('')
const error = ref('')
const msg = ref('')
const hint = ref('')
const panel = ref('')
const staged = ref({})

onMounted(load)

async function load() {
  error.value = ''
  try {
    const r = await api.nodes()
    items.value = r.items || []
    staged.value = r.agent_staged || {}
  } catch (e) {
    error.value = e.message
  }
}

async function create() {
  error.value = ''
  hint.value = ''
  try {
    const r = await api.createNode({ name: name.value })
    name.value = ''
    hint.value = r.install_hint || ''
    panel.value = r.panel || ''
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function pushOne(id) {
  msg.value = ''
  try {
    const r = await api.pushNodeUpdate(id)
    msg.value = r.message || '已标记推送'
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function pushAll() {
  if (!confirm('所有在线节点下次心跳都会从本面板拉新 agent。继续？')) return
  msg.value = ''
  try {
    const r = await api.pushAllNodeUpdates()
    msg.value = '已向全部节点标记更新 ' + (r.desired_version || '')
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function remove(id) {
  if (!confirm('删除该节点？')) return
  await api.deleteNode(id)
  await load()
}

function seen(n) {
  if (!n.last_seen) return '从未上线'
  return new Date(n.last_seen).toLocaleString()
}

async function copy(text) {
  await navigator.clipboard.writeText(text)
  msg.value = '已复制安装命令'
}
</script>

<template>
  <div>
    <div class="flex items-end justify-between mb-6">
      <div>
        <h2 class="font-display text-3xl">节点 / Agent</h2>
        <p class="text-ink/50 text-sm mt-1">节点从本面板拉二进制。点「推送更新」后，下次心跳会自动换包重启。</p>
      </div>
      <button class="btn-primary" @click="pushAll">一键推送全部 Agent</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>
    <p v-if="msg" class="text-pine text-sm mb-3">{{ msg }}</p>
    <p class="text-xs text-ink/40 mb-4">
      已暂存 agent：{{ Object.keys(staged).length ? Object.keys(staged).join(', ') : '无（先在设置里更新面板，或把 hallo-agent 放到数据目录 agents/）' }}
    </p>

    <form class="card p-5 mb-6 flex gap-3 items-end" @submit.prevent="create">
      <div class="flex-1">
        <label class="label">节点名</label>
        <input class="input" v-model="name" required placeholder="hk-1" />
      </div>
      <button class="btn-primary h-10" type="submit">添加节点</button>
    </form>

    <div v-if="hint" class="card p-5 mb-6">
      <div class="text-xs text-ink/45 mb-2">在节点机以 root 执行</div>
      <pre class="text-xs whitespace-pre-wrap break-all bg-ink/[0.04] rounded-xl p-3">{{ hint }}</pre>
      <button class="btn-ghost text-xs mt-3" @click="copy(hint)">复制</button>
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
