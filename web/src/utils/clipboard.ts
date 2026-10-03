// 剪贴板复制。
//
// Havline 常以 http://内网IP:6893 访问，属于**非安全上下文**，此时 navigator.clipboard 为 undefined，
// 直接调用会导致复制必然失败。这里统一收口：优先用异步剪贴板 API，不可用时回退到 execCommand。
// 返回是否复制成功，由调用方决定提示文案。
export async function copyText(text: string): Promise<boolean> {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 权限被拒或浏览器限制：继续尝试回退方案
    }
  }
  try {
    const el = document.createElement('textarea')
    el.value = text
    el.setAttribute('readonly', '')
    el.style.position = 'fixed'
    el.style.top = '-1000px'
    el.style.opacity = '0'
    document.body.appendChild(el)
    el.select()
    el.setSelectionRange(0, el.value.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(el)
    return ok
  } catch {
    return false
  }
}
