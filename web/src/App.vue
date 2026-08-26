<script setup>
import { toasts } from './toast'
</script>

<template>
  <router-view />
  <div class="toast-stack">
    <div
      v-for="t in toasts"
      :key="t.id"
      class="toast"
      :class="t.type === 'err' ? 'toast-err' : 'toast-ok'"
    >
      {{ t.message }}
    </div>
  </div>
</template>
