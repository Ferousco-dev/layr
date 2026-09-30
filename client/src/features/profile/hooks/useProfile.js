import { useCallback, useEffect, useState } from 'react'
import { ApiError, getProfile } from '../../../lib/api'
import { recall, remember } from '../../../lib/cache.js'

// useProfile loads the profile once and lets key rows update their own entry without a reload.
export function useProfile() {
  const cached = recall('profile')
  const [state, setState] = useState(cached ? { status: 'ready', profile: cached, error: '' } : { status: 'loading', profile: null, error: '' })

  const load = useCallback(async () => {
    setState((s) => (s.profile ? s : { ...s, status: 'loading', error: '' }))
    try {
      const profile = await getProfile()
      remember('profile', profile)
      setState({ status: 'ready', profile, error: '' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return window.location.reload()
      setState((s) => (s.profile ? s : { status: 'error', profile: null, error: err.message }))
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const setKey = useCallback((next) => {
    setState((s) => {
      const profile = { ...s.profile, ai_keys: s.profile.ai_keys.map((k) => (k.provider === next.provider ? next : k)) }
      remember('profile', profile)
      return { ...s, profile }
    })
  }, [])

  return { ...state, reload: load, setKey }
}
