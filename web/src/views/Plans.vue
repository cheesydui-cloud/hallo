<script setup>
import { onMounted, ref } from 'vue'
import { api, formatBytes } from '../api'
import { toastOk, toastErr } from '../toast'

const items = ref([])
const error = ref('')
const form = ref({ name: '', traffic_limit: 0, duration_days: 0, note: '' })

onMounted(load)

async function load() {
  try {
    const r = await api.plans()
    items.value = r.items || []
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function create() {
  error.value = ''
  try {
    await api.createPlan({
      name: form.value.name,
      traffic_limit: Number(form.value.traffic_limit) || 0,
      duration_days: Number(form.value.duration_days) || 0,
      note: form.value.note,
    })
    form.value = { name: '', traffic_limit: 0, duration_days: 0, note: '' }
    toastOk('套餐已添加')
    await load()
  } catch (e) {
    error.value = e.message
    toastErr(e)
  }
}

async function save(p) {
  try {
    await api.updatePlan(p.id, p)
    toastOk('套餐已保存')
    await load()
  } catch (e) {
    toastErr(e)
  }
}

async function remove(id) {
  if (!confirm('删除套餐？已绑定用户会变成无套餐。')) return
  try {
    await api.deletePlan(id)
    toastOk('套餐已删除')
    await load()
  } catch (e) {
    toastErr(e)
  }
}
</script>

<template>
  <div>
    <h2 class="font-display text-3xl mb-2">套餐</h2>
    <p class="text-ink/50 text-sm mb-6">流量上限 0 表示不限；有效天数 0 表示不过期。对应 CLI：hallo plan add</p>
    <p v-if="error" class="text-red-700 text-sm mb-3">{{ error }}</p>

    <form class="card p-5 mb-6 grid md:grid-cols-5 gap-3 items-end" @submit.prevent="create">
      <div>
        <label class="label">名称</label>
        <input class="input" v-model="form.name" required placeholder="admin" />
      </div>
      <div>
        <label class="label">流量上限（字节，0 不限）</label>
        <input class="input" v-model.number="form.traffic_limit" />
      </div>
      <div>
        <label class="label">天数（0 不限）</label>
        <input class="input" v-model.number="form.duration_days" />
      </div>
      <div>
        <label class="label">备注</label>
        <input class="input" v-model="form.note" placeholder="admin自用" />
      </div>
      <button class="btn-primary h-10" type="submit">添加套餐</button>
    </form>

    <div class="space-y-3">
      <div v-for="p in items" :key="p.id" class="card p-5 grid md:grid-cols-5 gap-3 items-end">
        <div>
          <label class="label">名称</label>
          <input class="input" v-model="p.name" />
        </div>
        <div>
          <label class="label">流量上限</label>
          <input class="input" v-model.number="p.traffic_limit" />
          <div class="text-[11px] text-ink/35 mt-1">{{ p.traffic_limit ? formatBytes(p.traffic_limit) : '不限' }}</div>
        </div>
        <div>
          <label class="label">天数</label>
          <input class="input" v-model.number="p.duration_days" />
        </div>
        <div>
          <label class="label">备注</label>
          <input class="input" v-model="p.note" />
        </div>
        <div class="flex gap-2">
          <button class="btn-primary flex-1" @click="save(p)">保存</button>
          <button class="btn-ghost" @click="remove(p.id)">删</button>
        </div>
      </div>
    </div>
  </div>
</template>
