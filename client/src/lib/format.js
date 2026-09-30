const UNITS = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
]

// timeAgo turns an ISO time into text like "2 hours ago".
export function timeAgo(iso, now = Date.now()) {
  const seconds = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (Number.isNaN(seconds)) return ''
  for (const [unit, size] of UNITS) {
    if (seconds >= size) {
      const n = Math.floor(seconds / size)
      return `${n} ${unit}${n === 1 ? '' : 's'} ago`
    }
  }
  return 'just now'
}

// asFigmaUrl accepts a pasted Figma design link (with or without https://) and returns it, or '' when it is not one.
export function asFigmaUrl(text) {
  const value = text.trim()
  if (!value || /\s/.test(value)) return ''
  try {
    const url = new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`)
    const host = url.hostname.toLowerCase()
    if (host !== 'figma.com' && host !== 'www.figma.com') return ''
    return url.href
  } catch {
    return ''
  }
}

// projectNameFromUrl guesses a readable name from the link until Figma reports the real file name.
export function projectNameFromUrl(figmaUrl) {
  try {
    const parts = new URL(figmaUrl).pathname.split('/').filter(Boolean)
    const slug = decodeURIComponent(parts[2] || '').replace(/[-_+]+/g, ' ').trim()
    return (slug || 'Untitled design').slice(0, 120)
  } catch {
    return 'Untitled design'
  }
}

export function initials(name) {
  return (name || '?').trim().charAt(0).toUpperCase()
}
