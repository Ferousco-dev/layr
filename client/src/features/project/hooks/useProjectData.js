import { useCallback, useEffect, useState } from 'react'
import { ApiError, getProject, loadDesign, loadTokens } from '../../../lib/api'

// useProjectData loads a project with its design and tokens for the preview page.
export function useProjectData(projectId) {
  const [state, setState] = useState({ status: 'loading', project: null, design: null, tokens: null, error: '' })

  const load = useCallback(async (quiet = false) => {
    if (!quiet) setState({ status: 'loading', project: null, design: null, tokens: null, error: '' })
    try {
      const project = await getProject(projectId)
      const design = await loadDesign(projectId, { fresh: true })
      const tokens = design ? await loadTokens(projectId) : null
      setState({ status: 'ready', project, design, tokens, error: '' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return window.location.reload()
      if (quiet) return
      setState({ status: err instanceof ApiError && err.status === 404 ? 'missing' : 'error', project: null, design: null, tokens: null, error: err.message })
    }
  }, [projectId])

  useEffect(() => {
    load()
  }, [load])

  return { ...state, reload: () => load(), refreshQuietly: () => load(true) }
}
