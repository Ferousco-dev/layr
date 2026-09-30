import { request } from './http.js'

// screenIds is a list of screens to use, or null for every screen.
export const startGeneration = async (projectId, provider, screenIds) =>
  (await request('POST', `/api/v1/projects/${projectId}/generations`, screenIds ? { provider, screen_ids: screenIds } : { provider })).data
export const getGeneration = async (projectId, generationId) => (await request('GET', `/api/v1/projects/${projectId}/generations/${generationId}`)).data
