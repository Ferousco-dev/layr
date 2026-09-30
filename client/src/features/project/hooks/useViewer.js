import { useCallback, useEffect, useRef, useState } from 'react'

const MIN = 0.25
const MAX = 3
const STEP = 0.25

const clamp = (value) => Math.min(MAX, Math.max(MIN, value))

// useViewer holds zoom and pan for the design canvas: buttons, drag, keyboard and fullscreen.
export function useViewer(resetKey) {
  const [view, setView] = useState({ zoom: 1, x: 0, y: 0 })
  const [fullscreen, setFullscreen] = useState(false)
  const box = useRef(null)
  const drag = useRef(null)

  useEffect(() => setView({ zoom: 1, x: 0, y: 0 }), [resetKey])

  useEffect(() => {
    const sync = () => setFullscreen(Boolean(document.fullscreenElement))
    document.addEventListener('fullscreenchange', sync)
    return () => document.removeEventListener('fullscreenchange', sync)
  }, [])

  const zoomBy = useCallback((delta) => setView((v) => ({ ...v, zoom: clamp(v.zoom + delta) })), [])
  const reset = useCallback(() => setView({ zoom: 1, x: 0, y: 0 }), [])

  const toggleFullscreen = useCallback(async () => {
    try {
      if (document.fullscreenElement) await document.exitFullscreen()
      else await box.current.requestFullscreen()
    } catch {
      // Fullscreen can be blocked by the browser; the viewer keeps working without it.
    }
  }, [])

  const handlers = {
    onPointerDown: (e) => {
      drag.current = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y }
      e.currentTarget.setPointerCapture(e.pointerId)
    },
    onPointerMove: (e) => {
      if (!drag.current) return
      const { x, y, vx, vy } = drag.current
      setView((v) => ({ ...v, x: vx + e.clientX - x, y: vy + e.clientY - y }))
    },
    onPointerUp: () => (drag.current = null),
    onPointerCancel: () => (drag.current = null),
    onKeyDown: (e) => {
      const moves = { ArrowLeft: [-30, 0], ArrowRight: [30, 0], ArrowUp: [0, -30], ArrowDown: [0, 30] }
      if (e.key === '+' || e.key === '=') zoomBy(STEP)
      else if (e.key === '-') zoomBy(-STEP)
      else if (moves[e.key]) setView((v) => ({ ...v, x: v.x + moves[e.key][0], y: v.y + moves[e.key][1] }))
      else return
      e.preventDefault()
    },
  }

  return { view, fullscreen, box, handlers, zoomIn: () => zoomBy(STEP), zoomOut: () => zoomBy(-STEP), reset, toggleFullscreen, canZoomIn: view.zoom < MAX, canZoomOut: view.zoom > MIN }
}
