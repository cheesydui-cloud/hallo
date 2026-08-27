<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'
import { copyText } from '../copy'

const users = ref([])
const error = ref('')
const form = ref({ email: '', remark: '' })
const copied = ref('')

onMounted(load)

async function load() {
  error.value = ''
  try {
    const u = await api.users()
    users.value = u.items || []
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function create() {
  error.value = ''
  try {
    await api.createUser({
      email: form.value.email,
      remark: form.value.remark,
    })
    form.value.email = ''
    form.value.remark = ''
    toastOk('客户端已添加，协议页的「复制」会用第一个启用的 UUID')
    await load()
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function toggle(u) {
  try {
    await api.updateUser(u.id, { enabled: !u.enabled, email: u.email, remark: u.remark })
    toastOk(u.enabled ? '已停用' : '已启用')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function resetUUID(id) {
  if (!confirm('重置 UUID？旧链接会失效，需要重新复制。')) return
  try {
    await api.resetUUID(id)
    toastOk('UUID 已重置')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function remove(id) {
  if (users.value.length <= 1) {
    toastErr('至少保留一个客户端，VLESS / VMess 需要 UUID')
    return
  }
  if (!confirm('删除该客户端？')) return
  try {
    await api.deleteUser(id)
    toastOk('已删除')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function copy(text, key) {
  const ok = await copyText(text)
  if (ok) {
    copied.value = key
    setTimeout(() => {
      if (copied.value === key) copied.value = ''
    }, 1500)
  }
}

function vlessText(u) {
  if (u.vless_links && u.vless_links.length) return u.vless_links.join('\n')
  return u.vless_link || ''
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold mb-2">客户端</h2>
    <p class="text-black/45 text-sm mb-6">
      初始化会自动创建一个默认 UUID。平时在「协议」页点复制即可。这里只在你要多个 UUID 时用。
    </p>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>

    <form class="card p-5 mb-6 grid md:grid-cols-3 gap-3 items-end" @submit.prevent="create">
      <div>
        <label class="label">标识</label>
        <input class="input" v-model="form.email" required placeholder="alice" />
      </div>
      <div>
        <label class="label">备注</label>
        <input class="input" v-model="form.remark" placeholder="自己用" />
      </div>
      <button class="btn-primary h-10" type="submit">添加</button>
    </form>

    <div class="card overflow-x-auto">
      <table class="w-full text-sm min-w-[640px]">
        <thead class="bg-ink/[0.03] text-ink/50 text-left">
          <tr>
            <th class="px-4 py-3 font-medium">客户端</th>
            <th class="px-4 py-3 font-medium">UUID</th>
            <th class="px-4 py-3 font-medium">链接</th>
            <th class="px-4 py-3 font-medium"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id" class="border-t border-ink/5 align-top">
            <td class="px-4 py-3">
              <div class="font-medium">{{ u.email }}</div>
              <div class="text-xs text-ink/40">{{ u.remark || '—' }} · {{ u.active ? '可用' : '停用' }}</div>
            </td>
            <td class="px-4 py-3 text-[11px] font-mono break-all">{{ u.uuid }}</td>
            <td class="px-4 py-3 space-x-2 whitespace-nowrap">
              <button class="btn-ghost text-xs" type="button" @click="copy(vlessText(u), u.id + 'v')">
                {{ copied === u.id + 'v' ? '已复制' : '全部节点' }}
              </button>
              <button class="btn-ghost text-xs" type="button" @click="copy(u.sub_url, u.id + 's')">
                {{ copied === u.id + 's' ? '已复制' : '订阅' }}
              </button>
            </td>
            <td class="px-4 py-3 text-right space-x-1 whitespace-nowrap">
              <button class="btn-ghost text-xs" type="button" @click="toggle(u)">{{ u.enabled ? '停用' : '启用' }}</button>
              <button class="btn-ghost text-xs" type="button" @click="resetUUID(u.id)">重置 UUID</button>
              <button class="btn-ghost text-xs text-red-700" type="button" @click="remove(u.id)">删除</button>
            </td>
          </tr>
          <tr v-if="!users.length">
            <td colspan="4" class="px-4 py-10 text-center text-ink/40">还没有客户端</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
