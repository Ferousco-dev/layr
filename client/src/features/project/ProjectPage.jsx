import { useMemo, useState } from 'react'
import AppHeader from '../../components/AppHeader.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import { Link, navigate } from '../../app/router.jsx'
import { ApiError, deleteProject, restoreProject } from '../../lib/api'
import { forget } from '../../lib/cache.js'
import AddKeyDialog from './components/AddKeyDialog.jsx'
import ChooseModelDialog from './components/ChooseModelDialog.jsx'
import DetailsCard from './components/DetailsCard.jsx'
import ProjectSkeleton from './components/ProjectSkeleton.jsx'
import Thumbnails from './components/Thumbnails.jsx'
import Viewer from './components/Viewer.jsx'
import { useGenerateFlow } from './hooks/useGenerateFlow.js'
import { useRefresh } from './hooks/useRefresh.js'
import { useProjectData } from './hooks/useProjectData.js'
import './project.css'

function Message({ children }) {
  return <div className="project-message">{children}</div>
}

export default function ProjectPage({ projectId, user, signOut }) {
  const { status, project, design, tokens, error, reload, refreshQuietly } = useProjectData(projectId)
  const [version, setVersion] = useState(0)
  const refresh = useRefresh(projectId, () => {
    setPicked(null)
    setVersion((v) => v + 1)
    refreshQuietly()
  })
  const [index, setIndex] = useState(0)
  const [picked, setPicked] = useState(null)
  const [deleted, setDeleted] = useState(null)
  const [deleteError, setDeleteError] = useState('')

  const remove = async () => {
    try {
      await deleteProject(projectId)
      forget('projects')
      setDeleted(project)
    } catch (err) {
      setDeleteError(err instanceof ApiError ? err.message : 'Could not delete the project.')
    }
  }
  const undo = async () => {
    try {
      await restoreProject(projectId)
      forget('projects')
      setDeleted(null)
      reload()
    } catch (err) {
      setDeleteError(err instanceof ApiError ? err.message : 'Could not restore the project.')
    }
  }

  const screens = design ? design.screens : []
  // Everything is chosen until the person changes it; choosing every screen is sent as "all".
  const selected = useMemo(() => picked || new Set(screens.map((s) => s.id)), [picked, screens])
  const chosenIds = selected.size === screens.length ? null : screens.filter((s) => selected.has(s.id)).map((s) => s.id)
  const generate = useGenerateFlow(projectId, chosenIds)
  const toggle = (id) => {
    const next = new Set(selected)
    next.has(id) ? next.delete(id) : next.add(id)
    setPicked(next)
  }
  const screen = screens[Math.min(index, Math.max(screens.length - 1, 0))]

  let body
  if (deleted) {
    body = (
      <Message>
        <h1>“{deleted.name}” was deleted</h1>
        <p>You can undo this for the next 30 days.</p>
        {deleteError && <p className="problem">{deleteError}</p>}
        <div className="row">
          <button type="button" className="generate small" onClick={undo}>
            Undo
          </button>
          <button type="button" className="ghost" onClick={() => navigate('/')}>
            Back to projects
          </button>
        </div>
      </Message>
    )
  } else if (status === 'loading') {
    body = <ProjectSkeleton />
  } else if (status === 'missing') {
    body = (
      <Message>
        <h1>Project not found</h1>
        <p>It may have been deleted, or the link is wrong.</p>
        <Link className="generate small" to="/">
          Back to projects
        </Link>
      </Message>
    )
  } else if (status === 'error') {
    body = (
      <Message>
        <h1>Could not load this project</h1>
        <p>{error}</p>
        <button type="button" className="generate small" onClick={reload}>
          Try again
        </button>
      </Message>
    )
  } else if (!design || !screen) {
    body = (
      <Message>
        <h1>{project.name}</h1>
        <p>This project has no finished design yet. Paste a Figma link on the Projects page to import one.</p>
        <Link className="generate small" to="/">
          Back to projects
        </Link>
      </Message>
    )
  } else {
    body = (
      <div className="project-grid">
        <div className="left-column">
          <Viewer screens={screens} index={index} onIndex={setIndex} refresh={refresh} />
          <Thumbnails screens={screens} index={index} onIndex={setIndex} selected={selected} onToggle={toggle} onSelectAll={() => setPicked(new Set(screens.map((s) => s.id)))} onClear={() => setPicked(new Set())} />
        </div>
        <DetailsCard key={version} user={user} project={project} design={design} tokens={tokens} screen={screen} generate={generate} selectedCount={selected.size} onDelete={remove} />
      </div>
    )
  }

  return (
    <div className="page project-page">
      <AppHeader user={user} onSignOut={signOut} />
      <main className="project-main">
        <nav className="crumbs" aria-label="Breadcrumb">
          <Link to="/">Projects</Link>
          <span aria-hidden="true">›</span>
          <span>{project ? project.name : 'Project'}</span>
        </nav>
        {body}
      </main>
      <SiteFooter withBrand />

      {generate.phase === 'add' && <AddKeyDialog error={generate.error} onSave={generate.saveKeyAndStart} onCancel={generate.cancel} />}
      {generate.phase === 'choose' && <ChooseModelDialog keys={generate.saved} onChoose={generate.start} onCancel={generate.cancel} />}
    </div>
  )
}
