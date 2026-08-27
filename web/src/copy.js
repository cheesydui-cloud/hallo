import { toastOk, toastErr } from './toast'

export async function copyText(text, ok = '已复制') {
  if (!text) {
    toastErr('没有可复制的内容')
    return false
  }
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      toastOk(ok)
      return true
    }
  } catch {
    /* fallback */
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.left = '-9999px'
    document.body.appendChild(ta)
    ta.select()
    const okExec = document.execCommand('copy')
    document.body.removeChild(ta)
    if (okExec) {
      toastOk(ok)
      return true
    }
  } catch {
    /* show text */
  }
  window.prompt('复制失败，请手动复制：', text)
  toastErr('复制失败，已弹出文本框，请手动全选复制')
  return false
}
