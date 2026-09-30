import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, forgetDesigns, latestImport, refreshImport, selectFrames } from '../../../lib/api'
import { MAX_ALL_SCREENS } from '../../../lib/limits.js'

const POLL_MS = 1500
const MAX_POLLS = 200
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

// useRefresh runs the Figma import again for a project and reports progress until the new design is ready.
export function useRefresh(projectId, onDone) {
  const [state, setState] = useState({ phase: 'idle', message: '' })
  const alive = useRef(true)

  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
    }
  }, [])

  const run = useCallback(async () => {
    if (state.phase === 'working') return
    const say = (next) => alive.current && setState(next)
    say({ phase: 'working', message: 'Reading your Figma file again…' })
    try {
      await refreshImport(projectId)
      for (let i = 0; i < MAX_POLLS && alive.current; i++) {
        await sleep(POLL_MS)
        const imp = await latestImport(projectId)
        if (imp.status === 'awaiting_selection') {
          const frames = (imp.frames || []).length
          if (frames > MAX_ALL_SCREENS) throw new ApiError(0, 'TOO_MANY_SCREENS', `This file has ${frames} frames. Import it again from Projects to choose which ones.`)
          say({ phase: 'working', message: `Importing all ${frames} screens…` })
          await selectFrames(projectId, imp.id, { all: true })
        } else if (imp.status === 'completed') {
          forgetDesigns()
          say({ phase: 'idle', message: '' })
          if (alive.current) onDone()
          return
        } else if (imp.status === 'failed') {
          throw new ApiError(0, (imp.error && imp.error.code) || 'IMPORT_FAILED', (imp.error && imp.error.message) || 'The refresh failed.')
        } else {
          say({ phase: 'working', message: 'Importing your design…' })
        }
      }
      if (alive.current) throw new ApiError(0, 'IMPORT_TIMEOUT', 'The refresh is taking too long. Please try again.')
    } catch (err) {
      say({ phase: 'error', message: err instanceof ApiError ? err.message : 'The refresh did not finish. Please try again.' })
    }
  }, [projectId, state.phase, onDone])

  return { state, run, dismiss: useCallback(() => setState({ phase: 'idle', message: '' }), []) }
}
