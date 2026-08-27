<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'
import { copyText } from '../copy'

const items = ref([])
const nodes = ref([])
const xray = ref({ running: false, path: '', message: '' })
const error = ref('')
const nodeFilter = ref(0)
const showForm = ref(false)
const busy = ref(false)
const editing = ref(null)

const empty = () => ({
  id: 0,
  node_id: 0,
  remark: '',
  tag: 'vless-in',
  protocol: 'vless',
  listen: '0.0.0.0',
  port: 443,
  flow: 'xtls-rprx-vision',
  dest: 'www.microsoft.com:443',
  server_name: 'www.microsoft.com',
  private_key: '',
  public_key: '',
  short_id: '',
  enabled: true,
})
const form = ref(empty())

onMounted(load)

async function load() {
  error.value = ''
  try {
    const [ib, n] = await Promise.all([api.inbounds(), api.nodes()])
    items.value = ib.items || []
    xray.value = { running: ib.xray_running, path: ib.xray_path, message: ib.xray_message }
    nodes.value = n.items || []
    if (!form.value.node_id && nodes.value.length) {
      const local = nodes.value.find((x) => x.is_local) || nodes.value[0]
      form.value.node_id = local.id
    }
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

const filtered = computed(() => {
  if (!nodeFilter.value) return items.value
  return items.value.filter((i) => i.node_id === nodeFilter.value || i.node_id === 0)
})

function nodeName(id) {
  if (!id) return '全部节点'
  return nodes.value.find((n) => n.id === id)?.name || '#' + id
}

function openAdd() {
  const n = nodes.value.find((x) => x.is_local) || nodes.value[0]
  form.value = empty()
  if (n) {
    form.value.node_id = n.id
    form.value.port = n.port || 443
    form.value.remark = n.name
  }
  editing.value = null
  showForm.value = true
}

function openEdit(row) {
  form.value = { ...row }
  editing.value = row.id
  showForm.value = true
}

async function save() {
  busy.value = true
  try {
    const payload = { ...form.value, node_id: Number(form.value.node_id) || 0, port: Number(form.value.port) || 443 }
    const r = editing.value ? await api.updateInbound(editing.value, payload) : await api.createInbound(payload)
    if (r.warning) toastErr('已保存，但 Xray 未起来：' + r.warning)
    else toastOk(editing.value ? '入站已更新并下发' : '入站已添加并下发到节点')
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
  if (!confirm('重新生成该入站 Reality 密钥？订阅里的 pbk/sid 会变，客户端要重新拉订阅。')) return
  try {
    const r = await api.regenInboundKeys(row.id)
    toastOk(r.warning ? '密钥已更新，但 Xray 未起来：' + r.warning : '密钥已更新')
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
  try {
    const r = await api.reloadXray()
    toastOk(r.message || (r.running ? '本机 Xray 已重载' : '配置已写入'))
    await load()
  } catch (e) {
    toastErr(e)
  }
}
</script>

<template>
  <div>
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-4">
      <div>
        <h2 class="text-xl font-semibold">入站</h2>
        <p class="text-sm text-black/45 mt-1">每条入站绑定一个节点。远程节点由 agent 拉配置并跑 Xray，订阅用节点公网地址:端口。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button class="btn-ghost" type="button" @click="reloadXray">重载本机 Xray</button>
        <button class="btn-primary" type="button" @click="openAdd">添加入站</button>
      </div>
    </div>

    <div class="rounded-md border px-4 py-2.5 text-sm mb-4 flex flex-wrap gap-x-6 gap-y-1" :class="xray.running ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-amber-200 bg-amber-50 text-amber-900'">
      <span>本机 Xray：{{ xray.running ? '运行中' : (xray.message || '未启动') }}</span>
      <span class="font-mono text-xs opacity-70">{{ xray.path }}</span>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>

    <div class="flex items-center gap-3 mb-3">
      <label class="text-sm text-black/50">节点</label>
      <select class="input max-w-xs" v-model.number="nodeFilter">
        <option :value="0">全部节点</option>
        <option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.name }} {{ n.is_local ? '(本机)' : '' }}</option>
      </select>
    </div>

    <div class="card table-wrap">
      <table class="ui min-w-[960px]">
        <thead>
          <tr>
            <th>启用</th>
            <th>备注</th>
            <th>节点</th>
            <th>端口</th>
            <th>协议</th>
            <th>客户端</th>
            <th>SNI</th>
            <th>密钥</th>
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
              <div class="text-[11px] text-black/40 font-mono">{{ row.tag }}</div>
            </td>
            <td>
              <div>{{ nodeName(row.node_id) }}</div>
              <div class="text-[11px] text-black/40">{{ row.listen }}</div>
            </td>
            <td class="font-mono">{{ row.port }}</td>
            <td>
              <span class="badge bg-sky-50 text-sky-700">{{ (row.protocol || 'vless').toUpperCase() }}+Reality</span>
            </td>
            <td>{{ row.client_num ?? 0 }}</td>
            <td class="text-xs">{{ row.server_name }}</td>
            <td>
              <span class="badge" :class="row.keys_ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'">
                {{ row.keys_ok ? '正常' : '无效' }}
              </span>
            </td>
            <td class="text-right whitespace-nowrap space-x-1">
              <button class="btn-ghost text-xs" type="button" @click="copyText(row.public_key, '已复制 pbk')">pbk</button>
              <button class="btn-ghost text-xs" type="button" @click="openEdit(row)">编辑</button>
              <button class="btn-ghost text-xs" type="button" @click="regen(row)">换密钥</button>
              <button class="btn-danger text-xs" type="button" @click="remove(row)">删除</button>
            </td>
          </tr>
          <tr v-if="!filtered.length">
            <td colspan="9" class="text-center text-black/40 py-10">还没有入站。点右上角「添加入站」，并选择节点。</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="showForm" class="fixed inset-0 z-40 bg-black/40 flex items-start justify-center overflow-y-auto p-6" @click.self="showForm = false">
      <form class="bg-white rounded-lg shadow-xl w-full max-w-2xl p-6 space-y-4 my-8" @submit.prevent="save">
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{{ editing ? '编辑入站' : '添加入站' }}</h3>
          <button class="text-black/40" type="button" @click="showForm = false">×</button>
        </div>
        <div class="grid md:grid-cols-2 gap-3">
          <div class="md:col-span-2">
            <label class="label">节点（这条入站跑在哪台机器）</label>
            <select class="input" v-model.number="form.node_id" required>
              <option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.name }} {{ n.is_local ? '· 本机' : '· Agent' }} · {{ n.public_host || n.host || '未上报 IP' }}</option>
            </select>
          </div>
          <div>
            <label class="label">备注</label>
            <input class="input" v-model="form.remark" placeholder="洛杉矶-443" />
          </div>
          <div>
            <label class="label">Tag</label>
            <input class="input font-mono text-xs" v-model="form.tag" />
          </div>
          <div>
            <label class="label">监听</label>
            <input class="input" v-model="form.listen" />
          </div>
          <div>
            <label class="label">端口</label>
            <input class="input" type="number" v-model.number="form.port" />
          </div>
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
            <input class="input" v-model="form.flow" />
          </div>
          <div>
            <label class="label">Short ID</label>
            <input class="input font-mono text-xs" v-model="form.short_id" placeholder="留空自动生成" />
          </div>
          <div class="md:col-span-2">
            <label class="label">Private Key</label>
            <input class="input font-mono text-xs" v-model="form.private_key" placeholder="留空自动生成" />
          </div>
          <div class="md:col-span-2">
            <label class="label">Public Key（客户端 pbk）</label>
            <input class="input font-mono text-xs" v-model="form.public_key" placeholder="留空自动生成" />
          </div>
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
