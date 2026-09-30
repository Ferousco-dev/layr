import { startTransition, useEffect, useState } from 'react'
import { startProgress } from '../lib/progress.js'

let endNavigation = null

// finishNavigation closes the progress bar once the new page is on screen.
export function finishNavigation() {
  if (endNavigation) endNavigation()
  endNavigation = null
}

// usePath follows the address bar. Updates run as transitions, so React keeps the current page visible until the next one is ready.
export function usePath() {
  const [path, setPath] = useState(window.location.pathname)
  useEffect(() => {
    const sync = () => startTransition(() => setPath(window.location.pathname))
    window.addEventListener('popstate', sync)
    return () => window.removeEventListener('popstate', sync)
  }, [])
  return path
}

// navigate moves to another page inside the app, with the progress bar running until it appears.
export function navigate(to) {
  if (to === window.location.pathname) return
  finishNavigation()
  endNavigation = startProgress()
  setTimeout(finishNavigation, 10000)
  window.history.pushState(null, '', to)
  window.dispatchEvent(new PopStateEvent('popstate'))
  window.scrollTo(0, 0)
}

// Link navigates inside the app; a caller's own onClick runs first and never replaces the navigation.
export function Link({ to, children, onClick, ...rest }) {
  const go = (event) => {
    if (onClick) onClick(event)
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    navigate(to)
  }
  return (
    <a href={to} onClick={go} {...rest}>
      {children}
    </a>
  )
}
