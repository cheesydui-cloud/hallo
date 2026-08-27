<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'

const items = ref([])
const nodes = ref([])
const error = ref('')
const showForm = ref(false)
const busy = ref(false)
const editing = ref(null)

const empty = () => ({
  id: 0,
  node_id: 0,
  remark: '',
  tag: '',
  protocol: 'freedom',
  address: '',
  port: 0,
  uuid: '',
  flow: '',
  public_key: '',
  short_id: '',
  server_name: '',
  username: '',
  password: '',
  enabled: true,
  is_default: false,
})
const form = ref(empty())

onMounted(load)

async function load() {
  try {
    const [o, n] = await Promise.all([api.outbounds(), api.nodes()])
    items.value = o.items || []
    nodes.value = n.items || []
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

function nodeName(id) {
  if (!id) return '全部节点'
  return nodes.value.find((n) => n.id === id)?.name || '#' + id
}

function openAdd() {
  form.value = empty()
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
    const payload = { ...form.value, node_id: Number(form.value.node_id) || 0, port: Number(form.value.port) || 0 }
    if (editing.value) await api.updateOutbound(editing.value, payload)
    else await api.createOutbound(payload)
    toastOk('出站已保存并下发')
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
    await api.updateOutbound(row.id, { ...row, enabled: !row.enabled })
    toastOk(row.enabled ? '已停用' : '已启用')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function setDefault(row) {
  try {
    for (const o of items.value) {
      if (o.is_default && o.id !== row.id) {
        await api.updateOutbound(o.id, { ...o, is_default: false })
      }
    }
    await api.updateOutbound(row.id, { ...row, is_default: true, enabled: true })
    toastOk('已设为默认出站（入站流量走这条）')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function remove(row) {
  if (row.is_default) {
    toastErr('默认出站不能删除')
    return
  }
  if (!confirm('删除该出站？')) return
  try {
    await api.deleteOutbound(row.id)
    toastOk('已删除')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

const protoHint = {
  freedom: '本机直连出网',
  blackhole: '丢弃流量',
  vless: '转发到另一台 VLESS+Reality（链式）',
  socks: '走 SOCKS5 上游',
  http: '走 HTTP 代理上游',
}
</script>

<template>
  <div>
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-4">
      <div>
        <h2 class="text-xl font-semibold">出站</h2>
        <p class="text-sm text-black/45 mt-1">
          默认出站决定入站流量从哪出去。直连 = freedom；链式可加一条 VLESS 出站并设为默认，或在节点页选「转发到」。
        </p>
      </div>
      <button class="btn-primary" type="button" @click="openAdd">添加出站</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>

    <div class="card table-wrap">
      <table class="ui min-w-[860px]">
        <thead>
          <tr>
            <th>启用</th>
            <th>备注 / Tag</th>
            <th>协议</th>
            <th>作用节点</th>
            <th>目标</th>
            <th>默认</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in items" :key="row.id">
            <td><input type="checkbox" :checked="row.enabled" @change="toggle(row)" /></td>
            <td>
              <div class="font-medium">{{ row.remark || row.tag }}</div>
              <div class="text-[11px] font-mono text-black/40">{{ row.tag }}</div>
            </td>
            <td>
              <span class="badge bg-slate-100 text-slate-700">{{ row.protocol }}</span>
            </td>
            <td>{{ nodeName(row.node_id) }}</td>
            <td class="text-xs font-mono">
              <span v-if="row.address">{{ row.address }}:{{ row.port }}</span>
              <span v-else class="text-black/35">—</span>
            </td>
            <td>
              <span v-if="row.is_default" class="badge bg-blue-50 text-blue-700">默认</span>
            </td>
            <td class="text-right whitespace-nowrap space-x-1">
              <button v-if="!row.is_default" class="btn-ghost text-xs" type="button" @click="setDefault(row)">设为默认</button>
              <button class="btn-ghost text-xs" type="button" @click="openEdit(row)">编辑</button>
              <button v-if="!row.is_default" class="btn-danger text-xs" type="button" @click="remove(row)">删除</button>
            </td>
          </tr>
          <tr v-if="!items.length">
            <td colspan="7" class="text-center text-black/40 py-10">还没有出站</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="showForm" class="fixed inset-0 z-40 bg-black/40 flex items-start justify-center overflow-y-auto p-6" @click.self="showForm = false">
      <form class="bg-white rounded-lg shadow-xl w-full max-w-xl p-6 space-y-4 my-8" @submit.prevent="save">
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{{ editing ? '编辑出站' : '添加出站' }}</h3>
          <button class="text-black/40" type="button" @click="showForm = false">×</button>
        </div>
        <div class="grid md:grid-cols-2 gap-3">
          <div>
            <label class="label">协议</label>
            <select class="input" v-model="form.protocol">
              <option value="freedom">freedom（直连）</option>
              <option value="blackhole">blackhole（阻断）</option>
              <option value="vless">vless（链式 Reality）</option>
              <option value="socks">socks</option>
              <option value="http">http</option>
            </select>
            <p class="text-[11px] text-black/40 mt-1">{{ protoHint[form.protocol] }}</p>
          </div>
          <div>
            <label class="label">作用节点</label>
            <select class="input" v-model.number="form.node_id">
              <option :value="0">全部节点</option>
              <option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.name }}</option>
            </select>
          </div>
          <div>
            <label class="label">备注</label>
            <input class="input" v-model="form.remark" />
          </div>
          <div>
            <label class="label">Tag</label>
            <input class="input font-mono text-xs" v-model="form.tag" placeholder="自动" />
          </div>
          <template v-if="form.protocol === 'vless' || form.protocol === 'socks' || form.protocol === 'http'">
            <div>
              <label class="label">地址</label>
              <input class="input" v-model="form.address" required />
            </div>
            <div>
              <label class="label">端口</label>
              <input class="input" type="number" v-model.number="form.port" required />
            </div>
          </template>
          <template v-if="form.protocol === 'vless'">
            <div class="md:col-span-2">
              <label class="label">UUID</label>
              <input class="input font-mono text-xs" v-model="form.uuid" />
            </div>
            <div>
              <label class="label">SNI</label>
              <input class="input" v-model="form.server_name" />
            </div>
            <div>
              <label class="label">Public Key</label>
              <input class="input font-mono text-xs" v-model="form.public_key" />
            </div>
            <div>
              <label class="label">Short ID</label>
              <input class="input font-mono text-xs" v-model="form.short_id" />
            </div>
            <div>
              <label class="label">Flow</label>
              <input class="input" v-model="form.flow" />
            </div>
          </template>
          <template v-if="form.protocol === 'socks' || form.protocol === 'http'">
            <div>
              <label class="label">用户名（可选）</label>
              <input class="input" v-model="form.username" />
            </div>
            <div>
              <label class="label">密码</label>
              <input class="input" v-model="form.password" />
            </div>
          </template>
        </div>
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" v-model="form.enabled" />
          启用
        </label>
        <div class="flex justify-end gap-2">
          <button class="btn-ghost" type="button" @click="showForm = false">取消</button>
          <button class="btn-primary" type="submit" :disabled="busy">{{ busy ? '保存中…' : '保存' }}</button>
        </div>
      </form>
    </div>
  </div>
</template>
