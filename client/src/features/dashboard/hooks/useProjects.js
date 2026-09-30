import { useCallback, useEffect, useState } from 'react'
import { ApiError, listProjects } from '../../../lib/api'
import { recall, remember } from '../../../lib/cache.js'

// useProjects owns the project list: paging and quiet refresh.
export function useProjects() {
  const cached = recall('projects')
  const [projects, setProjects] = useState(cached ? cached.projects : null)
  const [cursor, setCursor] = useState(cached ? cached.cursor : null)
  const [error, setError] = useState('')

  const load = useCallback(async (after) => {
    try {
      const page = await listProjects(after)
      setProjects((prev) => {
        const next = after && prev ? [...prev, ...page.data] : page.data
        remember('projects', { projects: next, cursor: page.next_cursor || null })
        return next
      })
      setCursor(page.next_cursor || null)
      setError('')
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return window.location.reload()
      setError(err.message)
      setProjects((prev) => prev || [])
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  return { projects, cursor, error, load }
}
