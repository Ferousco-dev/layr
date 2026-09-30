import { useEffect } from 'react'
import { SITE_URL } from '../config/env.js'

function setContent(selector, attribute, value) {
  const el = document.head.querySelector(selector)
  if (el) el.setAttribute(attribute, value)
}

// usePageMeta keeps the tab title and search / share tags in step with the current page.
export function usePageMeta({ title, description, path = '/', indexable = true }) {
  useEffect(() => {
    document.title = title
    setContent('meta[name="description"]', 'content', description)
    setContent('meta[name="robots"]', 'content', indexable ? 'index, follow' : 'noindex, nofollow')
    setContent('link[rel="canonical"]', 'href', SITE_URL + path)
    setContent('meta[property="og:title"]', 'content', title)
    setContent('meta[property="og:description"]', 'content', description)
    setContent('meta[property="og:url"]', 'content', SITE_URL + path)
    setContent('meta[name="twitter:title"]', 'content', title)
    setContent('meta[name="twitter:description"]', 'content', description)
  }, [title, description, path, indexable])
}
