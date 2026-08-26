<script setup>
import { onMounted, ref } from 'vue'
import { api, formatBytes } from '../api'

const users = ref([])
const plans = ref([])
const error = ref('')
const form = ref({ email: '', remark: '', plan_id: null })
const copied = ref('')

onMounted(load)

async function load() {
  error.value = ''
  try {
    const [u, p] = await Promise.all([api.users(), api.plans()])
    users.value = u.items || []
    plans.value = p.items || []
    if (!form.value.plan_id && plans.value.length) form.value.plan_id = plans.value[0].id
  } catch (e) {
    error.value = e.message
  }
}

async function create() {
  error.value = ''
  try {
    await api.createUser({
      email: form.value.email,
      remark: form.value.remark,
      plan_id: form.value.plan_id || null,
    })
    form.value.email = ''
    form.value.remark = ''
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function toggle(u) {
  await api.updateUser(u.id, { enabled: !u.enabled, email: u.email, remark: u.remark, plan_id: u.plan_id })
  await load()
}

async function resetTraffic(id) {
  await api.resetTraffic(id)
  await load()
}

async function resetUUID(id) {
  if (!confirm('重置 UUID 和订阅 token？旧链接会失效。')) return
  await api.resetUUID(id)
  await load()
}

async function remove(id) {
  if (!confirm('删除该用户？')) return
  await api.deleteUser(id)
  await load()
}

async function copy(text, key) {
  await navigator.clipboard.writeText(text)
  copied.value = key
  setTimeout(() => {
    if (copied.value === key) copied.value = ''
  }, 1500)
}

function expire(u) {
  if (!u.expire_at) return '不限'
  return new Date(u.expire_at).toLocaleDateString()
}
</script>

<template>
  <div>
    <h2 class="font-display text-3xl mb-6">用户</h2>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>

    <form class="card p-5 mb-6 grid md:grid-cols-4 gap-3 items-end" @submit.prevent="create">
      <div>
        <label class="label">标识 / 邮箱</label>
        <input class="input" v-model="form.email" required placeholder="alice@local" />
      </div>
      <div>
        <label class="label">备注</label>
        <input class="input" v-model="form.remark" placeholder="自己用" />
      </div>
      <div>
        <label class="label">套餐</label>
        <select class="input" v-model.number="form.plan_id">
          <option :value="null">无</option>
          <option v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </div>
      <button class="btn-primary h-10" type="submit">添加用户</button>
    </form>

    <div class="card overflow-hidden">
      <table class="w-full text-sm">
        <thead class="bg-ink/[0.03] text-ink/50 text-left">
          <tr>
            <th class="px-4 py-3 font-medium">用户</th>
            <th class="px-4 py-3 font-medium">套餐 / 到期</th>
            <th class="px-4 py-3 font-medium">流量</th>
            <th class="px-4 py-3 font-medium">订阅</th>
            <th class="px-4 py-3 font-medium"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id" class="border-t border-ink/5 align-top">
            <td class="px-4 py-3">
              <div class="font-medium">{{ u.email }}</div>
              <div class="text-xs text-ink/40">{{ u.remark || '—' }} · {{ u.enabled ? '启用' : '停用' }}</div>
              <div class="text-[11px] text-ink/35 mt-1 break-all">{{ u.uuid }}</div>
            </td>
            <td class="px-4 py-3">
              {{ u.plan_name || '无' }}<br />
              <span class="text-xs text-ink/40">{{ expire(u) }}</span>
            </td>
            <td class="px-4 py-3">{{ formatBytes(u.traffic_up + u.traffic_down) }}</td>
            <td class="px-4 py-3 space-x-2">
              <button class="btn-ghost text-xs" @click="copy(u.vless_link, u.id + 'v')">
                {{ copied === u.id + 'v' ? '已复制' : 'VLESS' }}
              </button>
              <button class="btn-ghost text-xs" @click="copy(u.sub_url, u.id + 's')">
                {{ copied === u.id + 's' ? '已复制' : '订阅' }}
              </button>
              <button class="btn-ghost text-xs" @click="copy(u.clash_url, u.id + 'c')">
                {{ copied === u.id + 'c' ? '已复制' : 'Clash' }}
              </button>
            </td>
            <td class="px-4 py-3 text-right space-x-1 whitespace-nowrap">
              <button class="btn-ghost text-xs" @click="toggle(u)">{{ u.enabled ? '停用' : '启用' }}</button>
              <button class="btn-ghost text-xs" @click="resetTraffic(u.id)">清流量</button>
              <button class="btn-ghost text-xs" @click="resetUUID(u.id)">重置 UUID</button>
              <button class="btn-ghost text-xs text-red-700" @click="remove(u.id)">删除</button>
            </td>
          </tr>
          <tr v-if="!users.length">
            <td colspan="5" class="px-4 py-10 text-center text-ink/40">还没有用户</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
