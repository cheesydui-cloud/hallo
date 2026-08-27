<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, formatBytes } from '../api'
import { toastOk, toastErr } from '../toast'

const router = useRouter()
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
        <p class="text-black/45 text-sm mt-1">一台面板管多台服务器。每台机器装 Agent，跑官方 Xray；在「入站」里选服务器再添加协议。</p>
      </div>
      <button class="btn-primary" :disabled="busy" @click="reload">{{ busy ? '重载中…' : '重载本机 Xray' }}</button>
    </div>
    <p v-if="error" class="text-red-700 text-sm mb-4">{{ error }}</p>

    <ol class="card p-5 mb-6 grid md:grid-cols-4 gap-4 text-sm">
      <li>
        <div class="text-[#1677ff] font-semibold">1. 服务器</div>
        <p class="text-black/50 mt-1">添加洛杉矶 / 香港等机器，把安装命令拿到那台机用 root 执行。要看到「在线」且 Xray 运行中。</p>
        <button class="btn-ghost text-xs mt-2" type="button" @click="router.push('/nodes')">去添加</button>
      </li>
      <li>
        <div class="text-[#1677ff] font-semibold">2. 入站</div>
        <p class="text-black/50 mt-1">左边点那台服务器，右边添加 VLESS / VMess / Shadowsocks。同一台机端口不能重复。</p>
        <button class="btn-ghost text-xs mt-2" type="button" @click="router.push('/inbounds')">去配置</button>
      </li>
      <li>
        <div class="text-[#1677ff] font-semibold">3. 出站</div>
        <p class="text-black/50 mt-1">默认「直连」即可。只有要链式转发或走代理出口时才改。</p>
        <button class="btn-ghost text-xs mt-2" type="button" @click="router.push('/outbounds')">看出站</button>
      </li>
      <li>
        <div class="text-[#1677ff] font-semibold">4. 客户端</div>
        <p class="text-black/50 mt-1">加一个用户，复制订阅。没有用户，Xray 里没有 UUID，节点一定不通。</p>
        <button class="btn-ghost text-xs mt-2" type="button" @click="router.push('/users')">去添加</button>
      </li>
    </ol>

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
        <div class="text-xs text-ink/45">服务器</div>
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
