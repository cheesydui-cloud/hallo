<script setup>
import { onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const route = useRoute()
const ready = ref(false)
const version = ref('')

const nav = [
  { to: '/', label: '总览', icon: '▣' },
  { to: '/nodes', label: '服务器', icon: '⬡' },
  { to: '/inbounds', label: '入站', icon: '↓' },
  { to: '/outbounds', label: '出站', icon: '↑' },
  { to: '/users', label: '客户端', icon: '◉' },
  { to: '/plans', label: '套餐', icon: '▤' },
  { to: '/settings', label: '面板设置', icon: '⚙' },
]

onMounted(async () => {
  try {
    const m = await api.meta()
    version.value = m.version || ''
    if (m.setup_needed) {
      router.replace('/login')
      return
    }
    await api.dashboard()
    ready.value = true
  } catch {
    router.replace('/login')
  }
})

async function logout() {
  await api.logout()
  router.replace('/login')
}
</script>

<template>
  <div v-if="ready" class="min-h-screen flex bg-[#f0f2f5]">
    <aside class="w-[220px] shrink-0 bg-[#001529] text-white flex flex-col min-h-screen">
      <div class="px-5 py-5 border-b border-white/10">
        <div class="text-lg font-semibold tracking-wide">Hallo</div>
        <div class="text-[11px] text-white/45 mt-1">选服务器 · 加协议</div>
      </div>
      <nav class="flex-1 py-3">
        <router-link
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="flex items-center gap-3 px-5 py-2.5 text-sm text-white/70 hover:text-white hover:bg-white/5"
          :class="route.path === item.to ? 'bg-[#1677ff] text-white hover:bg-[#1677ff] hover:text-white' : ''"
        >
          <span class="w-4 text-center opacity-80">{{ item.icon }}</span>
          {{ item.label }}
        </router-link>
      </nav>
      <div class="px-5 py-4 border-t border-white/10 text-[11px] text-white/40 flex items-center justify-between">
        <span>{{ version || 'dev' }}</span>
        <button class="text-white/70 hover:text-white" @click="logout">退出</button>
      </div>
    </aside>
    <div class="flex-1 min-w-0">
      <header class="h-14 bg-white border-b border-black/5 px-6 flex items-center text-sm text-black/55">
        {{ nav.find((n) => n.to === route.path)?.label || 'Hallo' }}
      </header>
      <main class="p-6">
        <router-view />
      </main>
    </div>
  </div>
</template>
