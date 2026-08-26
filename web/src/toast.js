import { ref } from 'vue'

export const toasts = ref([])

let seq = 0

export function toast(message, type = 'ok') {
  const id = ++seq
  toasts.value = [...toasts.value, { id, message: String(message || ''), type }]
  setTimeout(() => {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }, 4200)
}

export function toastOk(message) {
  toast(message, 'ok')
}

export function toastErr(err) {
  toast(err?.message || err || '操作失败', 'err')
}

export async function withToast(okMsg, fn) {
  try {
    const r = await fn()
    toastOk(okMsg || r?.message || '已完成')
    return r
  } catch (e) {
    toastErr(e)
    throw e
  }
}
