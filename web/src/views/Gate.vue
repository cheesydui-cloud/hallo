<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const setupNeeded = ref(false)
const loading = ref(true)
const error = ref('')
const form = ref({
  username: 'admin',
  password: '',
  public_url: window.location.origin,
  port: 18443,
})

onMounted(async () => {
  try {
    const m = await api.meta()
    setupNeeded.value = m.setup_needed
    if (!m.setup_needed) {
      try {
        await api.dashboard()
        router.replace('/')
      } catch {
        /* stay on login */
      }
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
})

async function submit() {
  error.value = ''
  try {
    if (setupNeeded.value) {
      await api.setup(form.value)
    } else {
      await api.login({ username: form.value.username, password: form.value.password })
    }
    router.replace('/')
  } catch (e) {
    error.value = e.message
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-6">
    <div class="w-full max-w-md card p-8">
      <p class="text-accent tracking-[0.2em] text-xs font-semibold uppercase">Phase 1</p>
      <h1 class="font-display text-4xl mt-2">Hallo</h1>
      <p class="text-ink/60 mt-2 text-sm">
        {{ setupNeeded ? '第一次启动，创建管理员并写入默认 Reality 入站。' : '登录管理面板。' }}
      </p>

      <form class="mt-8 space-y-4" @submit.prevent="submit" v-if="!loading">
        <div>
          <label class="label">用户名</label>
          <input class="input" v-model="form.username" autocomplete="username" />
        </div>
        <div>
          <label class="label">密码</label>
          <input class="input" type="password" v-model="form.password" autocomplete="current-password" />
        </div>
        <template v-if="setupNeeded">
          <div>
            <label class="label">面板公网地址</label>
            <input class="input" v-model="form.public_url" placeholder="http://ip:18080" />
          </div>
          <div>
            <label class="label">Xray 入站端口（开发机建议 18443）</label>
            <input class="input" type="number" v-model.number="form.port" />
          </div>
        </template>
        <p v-if="error" class="text-sm text-red-700">{{ error }}</p>
        <button class="btn-primary w-full" type="submit">
          {{ setupNeeded ? '初始化并进入' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>
