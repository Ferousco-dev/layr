import { useCallback, useEffect, useState } from 'react'
import { currentUser, forgetDesigns, logout } from '../lib/api'
import { forgetAll } from '../lib/cache.js'

const HINT = 'layr.signedIn'

// wasSignedIn tells, before the session check returns, whether this browser was signed in last time.
export function wasSignedIn() {
  try {
    return window.localStorage.getItem(HINT) === '1'
  } catch {
    return false
  }
}

function rememberSignedIn(on) {
  try {
    if (on) window.localStorage.setItem(HINT, '1')
    else window.localStorage.removeItem(HINT)
  } catch {
    // Storage can be blocked; the hint is only a nicety.
  }
}

// One session check per page load; 'checking' lets the UI avoid flashing the wrong screen.
export function useSession() {
  const [session, setSession] = useState({ status: 'checking', user: null, unreachable: false })

  useEffect(() => {
    let active = true
    currentUser()
      .then((user) => {
        rememberSignedIn(Boolean(user))
        if (active) setSession({ status: user ? 'signed-in' : 'signed-out', user, unreachable: false })
      })
      .catch(() => active && setSession({ status: 'signed-out', user: null, unreachable: true }))
    return () => {
      active = false
    }
  }, [])

  const signOut = useCallback(async () => {
    await logout()
    rememberSignedIn(false)
    forgetAll()
    forgetDesigns()
    setSession({ status: 'signed-out', user: null, unreachable: false })
  }, [])

  return { ...session, signOut }
}
