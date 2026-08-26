<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'

const inb = ref(null)
const error = ref('')
const busy = ref(false)
const copied = ref('')

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
    inb.value = r
    if (r.warning) {
      toastErr('已保存，但 Xray 未起来：' + r.warning)
    } else {
      toastOk(r.xray_running ? '入站已保存，Xray 已重载' : '入站已保存（本机还没有 xray）')
    }
  } catch (e) {
    error.value = e.message
    toastErr(e)
  } finally {
    busy.value = false
  }
}

async function regen() {
  if (!confirm('重新生成 Reality 密钥和 Short ID？现有客户端都要重新拉订阅。')) return
  error.value = ''
  try {
    const r = await api.regenKeys()
    inb.value = r
    toastOk(r.warning ? '密钥已更新，但 Xray 未起来：' + r.warning : '密钥已更新，请让用户重新拉订阅')
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function use443() {
  if (!inb.value) return
  inb.value.port = 443
  await save()
}

async function copy(text, key) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = key
    toastOk('已复制')
    setTimeout(() => {
      if (copied.value === key) copied.value = ''
    }, 1500)
  } catch (e) {
    toastErr(e)
  }
}

const keysBad = computed(() => inb.value && inb.value.keys_ok === false)
const portLooksDev = computed(() => inb.value && inb.value.port === 18443 && !inb.value.dev)
</script>

<template>
  <div class="max-w-3xl">
    <h2 class="font-display text-3xl mb-2">入站</h2>
    <p class="text-ink/50 text-sm mb-6">全局面板共用一套 Reality 密钥。各节点的<strong>监听端口和公网地址</strong>在「节点」页改。dest 必须是真实可连的 TLS 站点。</p>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>

    <div v-if="inb" class="space-y-3 mb-4">
      <div v-if="keysBad" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
        当前密钥还是占位符，客户端连不上。点下面「重新生成密钥」，不需要本机先有 xray。
      </div>
      <div v-if="portLooksDev" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
        入站端口是 <strong>18443</strong>（开发机默认）。生产服务器一般用 <strong>443</strong>，且面板要以 root 跑。
        <button class="underline ml-2" type="button" @click="use443">改成 443 并保存</button>
      </div>
      <div class="rounded-xl border border-ink/10 bg-white px-4 py-3 text-sm flex flex-wrap gap-x-6 gap-y-1">
        <span>Xray：{{ inb.xray_running ? '运行中' : (inb.xray_message || '未启动') }}</span>
        <span class="text-ink/50 font-mono text-xs">{{ inb.xray_path }}</span>
      </div>
    </div>

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
          <input class="input font-mono text-xs" v-model="inb.short_id" />
        </div>
        <div class="md:col-span-2">
          <label class="label">Private Key（服务端）</label>
          <input class="input font-mono text-xs" v-model="inb.private_key" />
        </div>
        <div class="md:col-span-2">
          <div class="flex items-center justify-between">
            <label class="label">Public Key（客户端 pbk）</label>
            <button class="text-xs underline text-pine" type="button" @click="copy(inb.public_key, 'pub')">
              {{ copied === 'pub' ? '已复制' : '复制' }}
            </button>
          </div>
          <input class="input font-mono text-xs" v-model="inb.public_key" />
        </div>
      </div>
      <div class="flex flex-wrap gap-3">
        <button class="btn-primary" type="submit" :disabled="busy">{{ busy ? '保存中…' : '保存并重载' }}</button>
        <button class="btn-ghost" type="button" @click="regen">重新生成密钥</button>
      </div>
    </form>
  </div>
</template>
