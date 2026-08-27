<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { toastOk, toastErr } from '../toast'

const s = ref({ public_url: '', xray_path: '', listen: '', panel_host: '', version: '', repo: '' })
const up = ref(null)
const error = ref('')
const checking = ref(false)
const updating = ref(false)

onMounted(async () => {
  try {
    s.value = await api.settings()
    await check()
  } catch (e) {
    toastErr(e)
  }
})

async function save() {
  error.value = ''
  try {
    await api.saveSettings(s.value)
    toastOk('已保存。公网地址立即用于订阅和节点拉包。')
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function check() {
  checking.value = true
  error.value = ''
  try {
    up.value = await api.updateStatus()
    toastOk(up.value.newer ? `有新版本 ${up.value.latest}` : `已是最新 ${up.value.latest || s.value.version}`)
  } catch (e) {
    error.value = e.message
    toastErr(e)
  } finally {
    checking.value = false
  }
}

async function apply() {
  if (!confirm('从 GitHub Release 拉取新版本并替换本机 hallo，随后重启服务。继续？')) return
  updating.value = true
  error.value = ''
  try {
    const r = await api.applyUpdate()
    toastOk(r.message || `已更新到 ${r.version || ''}，服务即将重启`)
    if (r.warning) toastErr(r.warning)
    setTimeout(() => location.reload(), 2500)
  } catch (e) {
    error.value = e.message
    toastErr(e)
  } finally {
    updating.value = false
  }
}
</script>

<template>
  <div class="max-w-xl space-y-6">
    <h2 class="text-xl font-semibold">面板设置</h2>

    <section class="card p-6 space-y-3">
      <div class="flex items-start justify-between gap-4">
        <div>
          <div class="text-xs text-ink/45">面板版本</div>
          <div class="text-2xl font-semibold mt-1">{{ s.version || 'dev' }}</div>
          <p class="text-xs text-ink/45 mt-1">更新从 GitHub Release 拉取，不走第三方 CDN。</p>
        </div>
        <button class="btn-ghost text-xs" :disabled="checking" @click="check">
          {{ checking ? '检查中…' : '检查更新' }}
        </button>
      </div>
      <div v-if="up" class="text-sm">
        <span v-if="up.newer">有新版本 <strong>{{ up.latest }}</strong></span>
        <span v-else>已是最新（{{ up.latest || s.version }}）</span>
        <a v-if="up.html_url" class="ml-2 underline text-pine" :href="up.html_url" target="_blank" rel="noreferrer">Release</a>
      </div>
      <button class="btn-primary" :disabled="updating || !up?.newer" @click="apply">
        {{ updating ? '更新中…' : '一键更新面板' }}
      </button>
    </section>

    <form class="card p-6 space-y-4" @submit.prevent="save">
      <div>
        <label class="label">公网地址（订阅主机名，也是 agent 拉包地址）</label>
        <input class="input" v-model="s.public_url" placeholder="http://1.2.3.4:18080" />
      </div>
      <div>
        <label class="label">Xray 可执行文件</label>
        <input class="input" v-model="s.xray_path" placeholder="/usr/local/bin/xray" />
      </div>
      <div>
        <label class="label">当前面板监听（只读）</label>
        <input class="input bg-ink/[0.03]" :value="s.listen" disabled />
      </div>
      <p v-if="error" class="text-red-700 text-sm">{{ error }}</p>
      <button class="btn-primary" type="submit">保存</button>
    </form>
  </div>
</template>
