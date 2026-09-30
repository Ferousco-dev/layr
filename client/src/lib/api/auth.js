import { API_URL } from '../../config/env.js'
import { request } from './http.js'

export const loginUrl = `${API_URL}/auth/figma`

// currentUser resolves to the signed-in user, null when signed out, and throws when the API cannot be reached.
export async function currentUser() {
  try {
    return (await request('GET', '/api/v1/me')).data
  } catch (err) {
    if (err.status === 401) return null
    throw err
  }
}

export async function logout() {
  try {
    await request('POST', '/auth/logout')
  } catch (err) {
    if (err.status !== 401) throw err
  }
}
