import { useCallback, useRef, useState } from 'react'
import AppHeader from '../../components/AppHeader.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import { asFigmaUrl } from '../../lib/format.js'
import ProjectCard from './components/ProjectCard.jsx'
import ProjectCardSkeleton from './components/ProjectCardSkeleton.jsx'
import SearchBar from './components/SearchBar.jsx'
import SelectScreens from './components/SelectScreens.jsx'
import Toast from './components/Toast.jsx'
import { useImporter } from './hooks/useImporter.js'
import { useProjects } from './hooks/useProjects.js'
import './dashboard.css'

export default function Dashboard({ user, signOut }) {
  const [query, setQuery] = useState('')
  const [view, setView] = useState('grid')
  const [toast, setToast] = useState(null)
  const [versions, setVersions] = useState({})
  const toastTimer = useRef(null)

  const say = useCallback((text, undo) => {
    clearTimeout(toastTimer.current)
    setToast(text ? { text, undo } : null)
    if (text) toastTimer.current = setTimeout(() => setToast(null), 8000)
  }, [])

  const { projects, cursor, error, load } = useProjects()

  const imported = useCallback(
    (projectId) => {
      setVersions((v) => ({ ...v, [projectId]: (v[projectId] || 0) + 1 }))
      load()
    },
    [load],
  )
  const importer = useImporter(imported)

  const submit = (event) => {
    event.preventDefault()
    const link = asFigmaUrl(query)
    if (link) {
      setQuery('')
      importer.start(link)
    } else if (/figma/i.test(query)) {
      importer.dismiss()
      say('That does not look like a Figma design link. Paste a link that starts with figma.com/design.')
    }
  }

  const looksLikeLink = Boolean(asFigmaUrl(query))
  const busy = importer.state.phase === 'working' || importer.state.phase === 'selecting'
  const needle = query.trim().toLowerCase()
  const shown = (projects || []).filter((p) => looksLikeLink || !needle || p.name.toLowerCase().includes(needle))

  return (
    <div className="page dashboard">
      <AppHeader user={user} onSignOut={signOut} />

      <main className="dash">
        <section className="hero-dash">
          <h1>
            Find a design, <span>turn it into code.</span>
          </h1>
          <p>Search your projects or paste a Figma URL to get started.</p>
          <SearchBar query={query} onChange={setQuery} onSubmit={submit} busy={busy} looksLikeLink={looksLikeLink} importState={importer.state} onDismiss={importer.dismiss} />
        </section>

        <section aria-label="Your projects">
          <div className="toolbar">
            <nav className="filters" aria-label="Project filters">
              <button type="button" className="active" aria-pressed="true">
                All projects{projects ? ` · ${projects.length}` : ''}
              </button>
            </nav>
            <div className="view-toggle" role="group" aria-label="Project view">
              <button type="button" className={view === 'grid' ? 'active' : ''} aria-pressed={view === 'grid'} onClick={() => setView('grid')}>
                <svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M3 3h6v6H3zm12 0h6v6h-6zM3 15h6v6H3zm12 0h6v6h-6z" />
                </svg>
                Grid
              </button>
              <button type="button" className={view === 'list' ? 'active' : ''} aria-pressed={view === 'list'} onClick={() => setView('list')}>
                <svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M8 5h13M8 12h13M8 19h13M3 5h1M3 12h1M3 19h1" />
                </svg>
                List
              </button>
            </div>
          </div>

          {error && (
            <p className="empty">
              {error}{' '}
              <button type="button" className="link" onClick={() => load()}>
                Try again
              </button>
            </p>
          )}
          {projects && !error && projects.length === 0 && <p className="empty">No projects yet. Paste a Figma design link above to import your first one.</p>}
          {projects && projects.length > 0 && shown.length === 0 && <p className="empty">No projects found. Try a different search.</p>}

          <div className={view === 'list' ? 'projects list' : 'projects'} aria-busy={projects === null}>
            {projects === null && Array.from({ length: 8 }, (_, i) => <ProjectCardSkeleton key={i} />)}
            {shown.map((p) => (
              <ProjectCard key={p.id} project={p} user={user} version={versions[p.id] || 0} />
            ))}
          </div>
          {cursor && (
            <p className="more">
              <button type="button" className="secondary" onClick={() => load(cursor)}>
                Load more
              </button>
            </p>
          )}
        </section>
      </main>

      <SiteFooter withBrand />

      {importer.state.phase === 'selecting' && <SelectScreens frames={importer.state.frames} fileName={importer.state.fileName} onConfirm={importer.choose} onCancel={importer.cancel} />}
      <Toast toast={toast} />
    </div>
  )
}
