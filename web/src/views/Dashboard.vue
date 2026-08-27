<script setup>
import { onMounted, ref } from 'vue'
import { api, formatBytes } from '../api'
import { toastOk, toastErr } from '../toast'

const d = ref(null)
const error = ref('')
const busy = ref(false)

onMounted(load)

async function load() {
  error.value = ''
  try {
    d.value = await api.dashboard()
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function reload() {
  busy.value = true
  try {
    const r = await api.reloadXray()
    toastOk(r.message || (r.running ? '本机 Xray 已重载' : '配置已写入，Xray 未运行'))
    await load()
  } catch (e) {
    toastErr(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-4 mb-6">
      <div>
        <h2 class="text-xl font-semibold">总览</h2>
        <p class="text-black/45 text-sm mt-1">入站按节点配置；出站决定流量从哪出去。远程节点由 hallo-agent 拉配置并跑官方 Xray。</p>
      </div>
      <button class="btn-primary" :disabled="busy" @click="reload">{{ busy ? '重载中…' : '重载本机 Xray' }}</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-4">{{ error }}</p>
    <div v-if="d" class="grid sm:grid-cols-2 lg:grid-cols-5 gap-4">
      <div class="card p-5">
        <div class="text-xs text-ink/45">用户</div>
        <div class="text-3xl font-semibold mt-2">{{ d.user_enabled }}/{{ d.user_total }}</div>
        <div class="text-xs text-ink/40 mt-1">启用 / 全部</div>
      </div>
      <div class="card p-5">
        <div class="text-xs text-ink/45">套餐</div>
        <div class="text-3xl font-semibold mt-2">{{ d.plan_total }}</div>
      </div>
      <div class="card p-5">
        <div class="text-xs text-ink/45">节点</div>
        <div class="text-3xl font-semibold mt-2">{{ d.node_online }}/{{ d.node_total }}</div>
        <div class="text-xs text-ink/40 mt-1">在线 / 全部</div>
      </div>
      <div class="card p-5">
        <div class="text-xs text-ink/45">累计流量</div>
        <div class="text-3xl font-semibold mt-2">{{ formatBytes(d.traffic_total) }}</div>
        <div class="text-xs text-ink/40 mt-1">精确 Stats 下一期</div>
      </div>
      <div class="card p-5">
        <div class="text-xs text-ink/45">本机 Xray</div>
        <div class="text-2xl font-semibold mt-2">{{ d.xray_running ? '运行中' : '未运行' }}</div>
        <div class="text-xs text-ink/40 mt-1">{{ d.xray_message }}</div>
        <div class="text-xs text-ink/40 mt-1">入站 :{{ d.inbound_port }}</div>
      </div>
    </div>
  </div>
</template>
