import { request } from './http.js'

export const startImport = async (projectId, figmaUrl) =>
  (await request('POST', `/api/v1/projects/${projectId}/import`, { figma_url: figmaUrl })).data
// refreshImport re-imports the project's Figma file; the server knows which file from the last import.
export const refreshImport = async (projectId) => (await request('POST', `/api/v1/projects/${projectId}/import/refresh`)).data
export const latestImport = async (projectId) => (await request('GET', `/api/v1/projects/${projectId}/import`)).data
export const selectFrames = async (projectId, importId, selection) =>
  (await request('POST', `/api/v1/projects/${projectId}/imports/${importId}/select`, selection)).data
