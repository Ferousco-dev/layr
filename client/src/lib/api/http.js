import { API_URL } from '../../config/env.js'
import { startProgress } from '../progress.js'

const TIMEOUT_MS = 8000

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message)
    this.status = status
    this.code = code
  }
}

// request sends a JSON call with the session cookie and turns failures into ApiError; the progress bar runs while it waits.
export async function request(method, path, body) {
  const done = startProgress()
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS)
  try {
    let res
    try {
      res = await fetch(API_URL + path, {
        method,
        credentials: 'include',
        signal: controller.signal,
        headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
    } catch {
      throw new ApiError(0, 'NETWORK', 'We cannot reach Layr right now. Please try again shortly.')
    }
    if (res.status === 204) return null
    const data = await res.json().catch(() => null)
    if (!res.ok) {
      const e = data && data.error
      throw new ApiError(res.status, (e && e.code) || 'ERROR', (e && e.message) || 'Something went wrong. Please try again.')
    }
    return data
  } finally {
    clearTimeout(timer)
    done()
  }
}

export const assetUrl = (path) => API_URL + path
