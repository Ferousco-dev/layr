import { request } from './http.js'

// Designs are cached per project; the promise is shared so a grid of cards asks once each.
const designs = new Map()
const resolved = new Map()

// peekDesign returns a design already loaded (null means none), or undefined when it is not known yet.
export const peekDesign = (projectId) => resolved.get(projectId)
export const forgetDesigns = () => {
  designs.clear()
  resolved.clear()
  tokens.clear()
  assets.clear()
}

export function loadDesign(projectId, { fresh = false } = {}) {
  if (fresh || !designs.has(projectId)) {
    designs.set(
      projectId,
      request('GET', `/api/v1/projects/${projectId}/design`)
        .then((res) => (res.data.status === 'ready' ? res.data : null))
        .then((design) => {
          resolved.set(projectId, design)
          return design
        })
        .catch((err) => {
          if ([404, 409, 410].includes(err.status)) {
            resolved.set(projectId, null)
            return null
          }
          designs.delete(projectId)
          throw err
        }),
    )
  }
  return designs.get(projectId)
}

// Tokens (colors, fonts, spacing) of a project's design; cached like the design itself.
const tokens = new Map()

export function loadTokens(projectId) {
  if (!tokens.has(projectId)) {
    tokens.set(
      projectId,
      request('GET', `/api/v1/projects/${projectId}/design/tokens`)
        .then((res) => res.data)
        .catch((err) => {
          tokens.delete(projectId)
          if ([404, 409, 410].includes(err.status)) return null
          throw err
        }),
    )
  }
  return tokens.get(projectId)
}


// Every image and vector Figma provided; cached like tokens.
const assets = new Map()

export function loadAssets(projectId) {
  if (!assets.has(projectId)) {
    assets.set(
      projectId,
      request('GET', `/api/v1/projects/${projectId}/design/assets`)
        .then((res) => res.data)
        .catch((err) => {
          assets.delete(projectId)
          if ([404, 409, 410].includes(err.status)) return []
          throw err
        }),
    )
  }
  return assets.get(projectId)
}

