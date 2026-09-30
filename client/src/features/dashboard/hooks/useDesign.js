import { useEffect, useRef, useState } from 'react'
import { loadDesign, peekDesign } from '../../../lib/api'

// useDesign loads a project's design summary once its element scrolls into view.
export function useDesign(projectId, version) {
  const ref = useRef(null)
  const [seen, setSeen] = useState(() => peekDesign(projectId) !== undefined)
  const [design, setDesign] = useState(() => peekDesign(projectId))

  useEffect(() => {
    const el = ref.current
    if (!el || seen) return
    if (!('IntersectionObserver' in window)) return setSeen(true)
    const observer = new IntersectionObserver(([entry]) => entry.isIntersecting && setSeen(true), { rootMargin: '200px' })
    observer.observe(el)
    return () => observer.disconnect()
  }, [seen])

  useEffect(() => {
    if (!seen) return
    let active = true
    loadDesign(projectId, { fresh: version > 0 })
      .then((d) => active && setDesign(d))
      .catch(() => active && setDesign(null))
    return () => {
      active = false
    }
  }, [seen, projectId, version])

  return { ref, design }
}
