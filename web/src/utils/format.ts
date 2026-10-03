const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const

export function formatBytes(bytes: number | null | undefined, fractionDigits = 1): string {
  const value = typeof bytes === 'number' && Number.isFinite(bytes) ? bytes : 0
  if (value <= 0) return '0 B'
  const exponent = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    BYTE_UNITS.length - 1,
  )
  const scaled = value / 1024 ** exponent
  const digits = exponent === 0 ? 0 : fractionDigits
  return `${scaled.toFixed(digits)} ${BYTE_UNITS[exponent]}`
}

export function formatDateTime(input?: string | null): string {
  if (!input) return '—'
  const date = new Date(input)
  if (Number.isNaN(date.getTime())) return '—'
  const pad = (part: number) => String(part).padStart(2, '0')
  return [
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
    `${pad(date.getHours())}:${pad(date.getMinutes())}`,
  ].join(' ')
}

export function formatNumber(value: number | null | undefined): string {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0)
}

export function formatMime(mime: string | null | undefined): string {
  if (!mime) return '未知'
  const sub = mime.split('/')[1] ?? mime
  return sub.replace('svg+xml', 'svg').replace('jpeg', 'jpg').split(';')[0].toUpperCase()
}

export function formatDimensions(width?: number | null, height?: number | null): string {
  if (!width || !height) return '—'
  return `${width} × ${height}`
}

export function usagePercent(used: number | null | undefined, quota: number | null | undefined): number {
  if (!quota || quota <= 0) return 0
  const ratio = ((used ?? 0) / quota) * 100
  return Math.min(100, Math.max(0, Math.round(ratio)))
}
