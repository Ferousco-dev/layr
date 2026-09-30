import { useCallback, useState } from 'react'
import { ApiError, getProfile, saveAiKey, startGeneration } from '../../../lib/api'
import { navigate } from '../../../app/router.jsx'
import { generationPath } from '../../../app/routes.js'

const LAST = 'layr.lastProvider'

const remember = (provider) => {
  try {
    window.localStorage.setItem(LAST, provider)
  } catch {
    // Remembering the last model is a nicety only.
  }
}
export const lastProvider = () => {
  try {
    return window.localStorage.getItem(LAST) || ''
  } catch {
    return ''
  }
}

// useGenerateFlow decides what happens when someone presses Generate: ask for a key, ask which model, or just start.
// screenIds is the chosen screens, or null when every screen is chosen.
export function useGenerateFlow(projectId, screenIds) {
  const [phase, setPhase] = useState('idle') // idle | checking | add | choose | starting
  const [saved, setSaved] = useState([])
  const [error, setError] = useState('')

  const fail = (err, fallback) => {
    setError(err instanceof ApiError ? err.message : fallback)
    setPhase('idle')
  }

  const start = useCallback(
    async (provider) => {
      setPhase('starting')
      setError('')
      try {
        const gen = await startGeneration(projectId, provider, screenIds)
        remember(provider)
        navigate(generationPath(projectId, gen.id))
      } catch (err) {
        if (err instanceof ApiError && err.code === 'KEY_REQUIRED') {
          setError(err.message)
          setPhase('add')
          return
        }
        fail(err, 'Could not start the generation. Please try again.')
      }
    },
    [projectId, screenIds],
  )

  const begin = useCallback(async () => {
    setPhase('checking')
    setError('')
    try {
      const profile = await getProfile()
      const keys = profile.ai_keys.filter((k) => k.saved)
      setSaved(keys)
      if (keys.length === 0) setPhase('add')
      else if (keys.length === 1) await start(keys[0].provider)
      else setPhase('choose')
    } catch (err) {
      fail(err, 'Could not check your API keys. Please try again.')
    }
  }, [start])

  const saveKeyAndStart = useCallback(
    async (provider, key) => {
      setError('')
      try {
        await saveAiKey(provider, key)
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Could not save the key.')
        return
      }
      await start(provider)
    },
    [start],
  )

  const cancel = useCallback(() => {
    setPhase('idle')
    setError('')
  }, [])

  return { phase, saved, error, begin, start, saveKeyAndStart, cancel, busy: phase === 'checking' || phase === 'starting' }
}
