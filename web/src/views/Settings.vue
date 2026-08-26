<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'

const s = ref({ public_url: '', xray_path: '', listen: '', panel_host: '' })
const msg = ref('')
const error = ref('')

onMounted(async () => {
  s.value = await api.settings()
})

async function save() {
  error.value = ''
  msg.value = ''
  try {
    await api.saveSettings(s.value)
    msg.value = '已保存。xray 路径立即生效，面板监听地址下次启动 --listen 才变。'
  } catch (e) {
    error.value = e.message
  }
}
</script>

<template>
  <div class="max-w-xl">
    <h2 class="font-display text-3xl mb-6">设置</h2>
    <form class="card p-6 space-y-4" @submit.prevent="save">
      <div>
        <label class="label">公网地址（写进订阅链接的主机名）</label>
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
