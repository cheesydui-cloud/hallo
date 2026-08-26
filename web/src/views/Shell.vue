<script setup>
import { onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const route = useRoute()
const ready = ref(false)

const nav = [
  { to: '/', label: '总览' },
  { to: '/users', label: '用户' },
  { to: '/plans', label: '套餐' },
  { to: '/inbound', label: '入站' },
  { to: '/nodes', label: '节点' },
  { to: '/settings', label: '设置' },
]

onMounted(async () => {
  try {
    const m = await api.meta()
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
  <div v-if="ready" class="min-h-screen">
    <header class="px-6 py-5 flex items-center gap-8">
      <div>
        <div class="font-display text-2xl leading-none">Hallo</div>
        <div class="text-[11px] tracking-widest uppercase text-ink/40 mt-1">自研单节点面板</div>
      </div>
      <nav class="flex gap-1 flex-1">
        <router-link
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="px-3 py-1.5 rounded-full text-sm"
          :class="route.path === item.to ? 'bg-ink text-paper' : 'text-ink/70 hover:bg-ink/5'"
        >
          {{ item.label }}
        </router-link>
      </nav>
      <button class="btn-ghost text-xs" @click="logout">退出</button>
    </header>
    <main class="px-6 pb-16">
      <router-view />
    </main>
  </div>
</template>
