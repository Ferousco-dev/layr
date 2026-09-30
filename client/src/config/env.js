// Build-time settings. The API and site origins never come from the page URL.
const env = import.meta.env ?? {}

export const API_URL = (env.VITE_API_URL || 'http://localhost:8080').replace(/\/+$/, '')
export const SITE_URL = (env.VITE_SITE_URL || 'https://layr.appmd.dev').replace(/\/+$/, '')

// Who runs Layr and how to reach them; shown in the legal pages and on the contact page.
export const OPERATOR_NAME = env.VITE_OPERATOR_NAME || 'Layr'
export const CONTACT_EMAIL = env.VITE_CONTACT_EMAIL || 'hello@layr.appmd.dev'
export const GOVERNING_LAW = env.VITE_GOVERNING_LAW || ''
