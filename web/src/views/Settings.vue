<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'

const s = ref({ public_url: '', xray_path: '', listen: '', panel_host: '', version: '', repo: '' })
const up = ref(null)
const msg = ref('')
const error = ref('')
const checking = ref(false)
const updating = ref(false)

onMounted(async () => {
  s.value = await api.settings()
  await check()
})

async function save() {
  error.value = ''
  msg.value = ''
  try {
    await api.saveSettings(s.value)
    msg.value = '已保存。xray 路径立即生效，面板监听地址下次启动才变。'
  } catch (e) {
    error.value = e.message
  }
}

async function check() {
  checking.value = true
  error.value = ''
  try {
    up.value = await api.updateStatus()
  } catch (e) {
    error.value = e.message
  } finally {
    checking.value = false
  }
}

async function apply() {
  if (!confirm('从 GitHub Release 拉取新版本并替换本机 hallo，随后重启服务。继续？')) return
  updating.value = true
  error.value = ''
  msg.value = ''
  try {
    const r = await api.applyUpdate()
    msg.value = r.message || `已更新到 ${r.version || ''}，服务即将重启，请稍候刷新。`
    if (r.warning) msg.value += ' ' + r.warning
    setTimeout(() => location.reload(), 2500)
  } catch (e) {
    error.value = e.message
  } finally {
    updating.value = false
  }
}
</script>

<template>
  <div class="max-w-xl space-y-6">
    <h2 class="font-display text-3xl">设置</h2>

    <section class="card p-6 space-y-3">
      <div class="flex items-start justify-between gap-4">
        <div>
          <div class="text-xs text-ink/45">面板版本</div>
          <div class="font-display text-2xl mt-1">{{ s.version || 'dev' }}</div>
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
      <p v-if="msg" class="text-pine text-sm">{{ msg }}</p>
      <p v-if="error" class="text-red-700 text-sm">{{ error }}</p>
      <button class="btn-primary" type="submit">保存</button>
    </form>
  </div>
</template>
