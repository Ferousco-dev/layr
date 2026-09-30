// Every page the app serves; anything else is a 404.
export const ROUTES = ['/', '/profile', '/docs', '/terms', '/privacy', '/contact']

const UUID = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
const PROJECT = new RegExp(`^/projects/(${UUID})$`)
const GENERATION = new RegExp(`^/projects/(${UUID})/generations/(${UUID})$`)

export const normalizePath = (pathname) => pathname.replace(/\/+$/, '') || '/'

// matchDynamic recognises pages whose address carries an id, and returns their parts.
export function matchDynamic(pathname) {
  const path = normalizePath(pathname)
  let m = GENERATION.exec(path)
  if (m) return { name: 'generation', projectId: m[1], generationId: m[2] }
  m = PROJECT.exec(path)
  if (m) return { name: 'project', projectId: m[1] }
  return null
}

export const isKnownRoute = (pathname) => ROUTES.includes(normalizePath(pathname)) || matchDynamic(pathname) !== null

export const projectPath = (projectId) => `/projects/${projectId}`
export const generationPath = (projectId, generationId) => `/projects/${projectId}/generations/${generationId}`
