export const APP_BASE = import.meta.env.BASE_URL.replace(/\/$/, '')

export function withBase(path: string): string {
  if (!path.startsWith('/')) return path
  return `${APP_BASE}${path}`
}
