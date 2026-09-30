import { useCallback, useEffect, useMemo, useState } from 'react'
import { ApiError, getGeneration } from '../../../lib/api'
import { eventsFrom, stepsAfter } from '../progress.js'

const POLL_MS = 700
const REVEAL_MS = 600
const OLD_MS = 15000

// useGenerationProgress follows a real job: it polls the server, then reveals each recorded change a moment apart so it can be read.
export function useGenerationProgress(projectId, generationId) {
  const [raw, setRaw] = useState(null)
  const [error, setError] = useState('')
  const [shown, setShown] = useState(0)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let active = true
    let timer
    let failures = 0
    const poll = async () => {
      try {
        const gen = await getGeneration(projectId, generationId)
        if (!active) return
        failures = 0
        setError('')
        setRaw((prev) => {
          if (!prev && gen.status !== 'running' && Date.now() - new Date(gen.updated_at).getTime() > OLD_MS) setShown(Number.MAX_SAFE_INTEGER)
          return gen
        })
        if (gen.status === 'running') timer = setTimeout(poll, POLL_MS)
      } catch (err) {
        if (!active) return
        if (err instanceof ApiError && err.status === 404) return setError('missing')
        failures += 1
        if (failures >= 4) return setError(err.message)
        timer = setTimeout(poll, POLL_MS * 2)
      }
    }
    poll()
    return () => {
      active = false
      clearTimeout(timer)
    }
  }, [projectId, generationId, attempt])

  const events = useMemo(() => (raw ? eventsFrom(raw.steps) : []), [raw])

  useEffect(() => {
    if (shown >= events.length) return
    const t = setTimeout(() => setShown((n) => Math.min(n + 1, events.length)), REVEAL_MS)
    return () => clearTimeout(t)
  }, [shown, events.length])

  const count = Math.min(shown, events.length)
  const steps = useMemo(() => (raw ? stepsAfter(raw.steps, events, count) : []), [raw, events, count])
  const finished = Boolean(raw) && raw.status !== 'running' && count >= events.length

  return { generation: raw, steps, finished, error, retry: useCallback(() => setAttempt((n) => n + 1), []) }
}
