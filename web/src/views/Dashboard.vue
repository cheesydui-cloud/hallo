<script setup>
import { onMounted, ref } from 'vue'
import { api, formatBytes } from '../api'

const d = ref(null)
const error = ref('')
const msg = ref('')

onMounted(load)

async function load() {
  error.value = ''
  try {
    d.value = await api.dashboard()
  } catch (e) {
    error.value = e.message
  }
}

async function reload() {
  msg.value = ''
  try {
    const r = await api.reloadXray()
    msg.value = r.message || '已重载'
    await load()
  } catch (e) {
    error.value = e.message
  }
}
</script>

<template>
  <div>
    <div class="flex items-end justify-between mb-6">
      <div>
        <h2 class="font-display text-3xl">总览</h2>
        <p class="text-ink/50 text-sm mt-1">第 1 期：管理用户、套餐，并把启用中的用户写进本机 Xray。</p>
      </div>
      <button class="btn-primary" @click="reload">重载 Xray</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-4">{{ error }}</p>
    <p v-if="msg" class="text-pine text-sm mb-4">{{ msg }}</p>
    <div v-if="d" class="grid md:grid-cols-4 gap-4">
      <div class="card p-5">
        <div class="text-xs text-ink/45">用户</div>
        <div class="font-display text-3xl mt-2">{{ d.user_enabled }}/{{ d.user_total }}</div>
        <div class="text-xs text-ink/40 mt-1">启用 / 全部</div>
      </div>
      <div class="card p-5">
        <div class="text-xs text-ink/45">套餐</div>
        <div class="font-display text-3xl mt-2">{{ d.plan_total }}</div>
      </div>
      <div class="card p-5">
        <div class="text-xs text-ink/45">累计流量</div>
        <div class="font-display text-3xl mt-2">{{ formatBytes(d.traffic_total) }}</div>
        <div class="text-xs text-ink/40 mt-1">第 2 期接 Stats 自动记账</div>
      </div>
      <div class="card p-5">
        <div class="text-xs text-ink/45">Xray</div>
        <div class="font-display text-2xl mt-2">{{ d.xray_running ? '运行中' : '未运行' }}</div>
        <div class="text-xs text-ink/40 mt-1">{{ d.xray_message }}</div>
        <div class="text-xs text-ink/40 mt-1">入站 :{{ d.inbound_port }}</div>
      </div>
    </div>
  </div>
</template>
