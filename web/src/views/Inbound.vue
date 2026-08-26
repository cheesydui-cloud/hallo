<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'

const inb = ref(null)
const error = ref('')
const busy = ref(false)

onMounted(load)

async function load() {
  error.value = ''
  try {
    inb.value = await api.inbound()
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function save() {
  error.value = ''
  busy.value = true
  try {
    const r = await api.saveInbound(inb.value)
    toastOk(r.warning ? '已保存，但重载失败：' + r.warning : '入站已保存并尝试重载 Xray')
    await load()
  } catch (e) {
    error.value = e.message
    toastErr(e)
  } finally {
    busy.value = false
  }
}

async function regen() {
  if (!confirm('重新生成 Reality 密钥？现有客户端都要换订阅。')) return
  error.value = ''
  try {
    inb.value = await api.regenKeys()
    toastOk('密钥已更新，请让用户重新拉订阅')
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}
</script>

<template>
  <div class="max-w-3xl">
    <h2 class="font-display text-3xl mb-2">入站</h2>
    <p class="text-ink/50 text-sm mb-6">第 1 期只做一条 VLESS + Reality。dest 必须是真实可连的 TLS 站点。</p>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>
    <form v-if="inb" class="card p-6 space-y-4" @submit.prevent="save">
      <div class="grid md:grid-cols-2 gap-4">
        <div>
          <label class="label">监听</label>
          <input class="input" v-model="inb.listen" />
        </div>
        <div>
          <label class="label">端口</label>
          <input class="input" type="number" v-model.number="inb.port" />
        </div>
        <div>
          <label class="label">Dest（回落目标）</label>
          <input class="input" v-model="inb.dest" />
        </div>
        <div>
          <label class="label">SNI / serverNames</label>
          <input class="input" v-model="inb.server_name" />
        </div>
        <div>
          <label class="label">Flow</label>
          <input class="input" v-model="inb.flow" />
        </div>
        <div>
          <label class="label">Short ID</label>
          <input class="input" v-model="inb.short_id" />
        </div>
        <div class="md:col-span-2">
          <label class="label">Private Key</label>
          <input class="input font-mono text-xs" v-model="inb.private_key" />
        </div>
        <div class="md:col-span-2">
          <label class="label">Public Key</label>
          <input class="input font-mono text-xs" v-model="inb.public_key" />
        </div>
      </div>
      <div class="flex gap-3">
        <button class="btn-primary" type="submit" :disabled="busy">{{ busy ? '保存中…' : '保存并重载' }}</button>
        <button class="btn-ghost" type="button" @click="regen">用 xray 重新生成密钥</button>
      </div>
    </form>
  </div>
</template>
