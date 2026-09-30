import { request } from './http.js'

// The phrase the backend requires before it deletes an account.
export const DELETE_PHRASE = 'delete my account'

export const getProfile = async () => (await request('GET', '/api/v1/profile')).data
export const saveAiKey = async (provider, apiKey) => (await request('PUT', `/api/v1/profile/ai-keys/${provider}`, { api_key: apiKey })).data
export const deleteAiKey = (provider) => request('DELETE', `/api/v1/profile/ai-keys/${provider}`)
export const deleteAccount = () => request('DELETE', '/api/v1/profile', { confirm: DELETE_PHRASE })
