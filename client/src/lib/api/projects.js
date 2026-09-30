import { request } from './http.js'

export const getProject = async (id) => (await request('GET', `/api/v1/projects/${id}`)).data
export const listProjects = (cursor) =>
  request('GET', `/api/v1/projects?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`)
export const createProject = async (name) => (await request('POST', '/api/v1/projects', { name })).data
export const renameProject = async (id, name) => (await request('PATCH', `/api/v1/projects/${id}`, { name })).data
export const deleteProject = (id) => request('DELETE', `/api/v1/projects/${id}`)
export const restoreProject = async (id) => (await request('POST', `/api/v1/projects/${id}/restore`)).data
