import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { progressCount, subscribeProgress } from '../lib/progress.js'
import './progress.css'

const SHOW_AFTER_MS = 120

// TopProgress is the slim bar at the top of the page; it appears only if work takes longer than a blink.
export default function TopProgress() {
  const busy = useSyncExternalStore(subscribeProgress, progressCount) > 0
  const [bar, setBar] = useState({ visible: false, width: 0 })
  const timers = useRef([])

  useEffect(() => {
    const clear = () => {
      timers.current.forEach((t) => clearTimeout(t) || clearInterval(t))
      timers.current = []
    }
    clear()
    if (busy) {
      timers.current.push(
        setTimeout(() => {
          setBar({ visible: true, width: 25 })
          timers.current.push(setInterval(() => setBar((b) => ({ ...b, width: b.width + (92 - b.width) * 0.12 })), 250))
        }, SHOW_AFTER_MS),
      )
    } else {
      setBar((b) => (b.visible ? { visible: true, width: 100 } : b))
      timers.current.push(setTimeout(() => setBar({ visible: false, width: 0 }), 300))
    }
    return clear
  }, [busy])

  return (
    <div className={bar.visible ? 'top-progress on' : 'top-progress'} style={{ width: `${bar.width}%` }} role="progressbar" aria-hidden={!bar.visible} aria-label="Loading" />
  )
}
