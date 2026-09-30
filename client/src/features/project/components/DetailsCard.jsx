import { useState } from 'react'
import { assetUrl } from '../../../lib/api'
import { timeAgo } from '../../../lib/format.js'
import { initials } from '../../../lib/format.js'
import Loader from '../../../components/Loader.jsx'
import AssetsTab from './AssetsTab.jsx'
import { Colors, Fonts } from './Palette.jsx'
import TokensTab from './TokensTab.jsx'

const TABS = [
  { id: 'overview', label: 'Overview' },
  { id: 'tokens', label: 'Design tokens' },
  { id: 'assets', label: 'Assets' },
]

function Overview({ project, design, screen }) {
  const pages = new Set(design.screens.map((s) => s.page).filter(Boolean)).size
  const c = design.counts
  const rows = [
    ['Type', 'Figma file'],
    ['File', design.source && design.source.file_name ? design.source.file_name : 'Untitled'],
    ['Pages', pages ? `${pages} ${pages === 1 ? 'page' : 'pages'}` : 'Not named'],
    ['Screens', `${c.screens}${c.flows ? `, in ${c.flows} ${c.flows === 1 ? 'flow' : 'flows'}` : ''}`],
    ['Dimensions', `${Math.round(screen.width)} × ${Math.round(screen.height)}`],
    ['Last updated', new Date(project.updated_at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })],
  ]
  if (c.warnings) rows.push(['Notes', `${c.warnings} ${c.warnings === 1 ? 'item' : 'items'} Layr could not fully read`])
  return (
    <dl className="metadata">
      {rows.map(([k, v]) => (
        <div key={k}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </div>
      ))}
    </dl>
  )
}

export default function DetailsCard({ user, project, design, tokens, screen, generate, selectedCount, onDelete }) {
  const [tab, setTab] = useState('overview')
  const [notice, setNotice] = useState('')
  const [confirming, setConfirming] = useState(false)
  const thumb = design.screens[0] && design.screens[0].preview && design.screens[0].preview.available ? assetUrl(design.screens[0].preview.url) : ''

  const onKey = (e, i) => {
    const next = { ArrowRight: (i + 1) % TABS.length, ArrowLeft: (i + TABS.length - 1) % TABS.length, Home: 0, End: TABS.length - 1 }[e.key]
    if (next === undefined) return
    e.preventDefault()
    setTab(TABS[next].id)
    document.getElementById(`tab-${TABS[next].id}`)?.focus()
  }

  return (
    <aside className="details-card">
      <div className="project-heading">
        {thumb ? <img src={thumb} alt="" /> : <span className="thumb-empty big" />}
        <div>
          <h1>{project.name}</h1>
          <p className="author">
            {user.avatar_url ? <img src={user.avatar_url} alt="" referrerPolicy="no-referrer" /> : <span className="avatar-fallback">{initials(user.name)}</span>}
            By {user.name}
          </p>
          <p className="updated">Last updated {timeAgo(project.updated_at)}</p>
        </div>
      </div>

      <div className="tabs" role="tablist" aria-label="Project details">
        {TABS.map((t, i) => (
          <button key={t.id} type="button" role="tab" id={`tab-${t.id}`} aria-controls={`panel-${t.id}`} aria-selected={tab === t.id} tabIndex={tab === t.id ? 0 : -1} onClick={() => setTab(t.id)} onKeyDown={(e) => onKey(e, i)}>
            {t.label}
          </button>
        ))}
      </div>
      <div className="tab-panel" role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}>
        {tab === 'overview' && <Overview project={project} design={design} screen={screen} />}
        {tab === 'tokens' && <TokensTab tokens={tokens} />}
        {tab === 'assets' && <AssetsTab projectId={project.id} screens={design.screens} />}
      </div>

      {tokens && <Colors colors={tokens.colors} onNotice={setNotice} />}
      {tokens && <Fonts fonts={tokens.fonts} />}

      <button type="button" className="generate" onClick={generate.begin} disabled={generate.busy || selectedCount === 0}>
        {generate.busy ? (
          <Loader size={26} />
        ) : (
          <>
            <span aria-hidden="true">〈/〉</span>
            {selectedCount === design.screens.length ? 'Generate Code' : `Generate from ${selectedCount} ${selectedCount === 1 ? 'screen' : 'screens'}`}
            <span aria-hidden="true">→</span>
          </>
        )}
      </button>
      <p className={generate.error ? 'status error' : 'status'} role="status">
        {generate.error || notice || (selectedCount === 0 ? 'Select at least one screen to generate from.' : 'Layr uses your own AI key to write the code.')}
      </p>

      <div className="danger-row">
        {confirming ? (
          <span>
            Delete this project?{' '}
            <button type="button" className="danger" onClick={onDelete}>
              Yes, delete
            </button>{' '}
            <button type="button" className="ghost" onClick={() => setConfirming(false)}>
              Keep
            </button>
          </span>
        ) : (
          <button type="button" className="ghost" onClick={() => setConfirming(true)}>
            Delete project
          </button>
        )}
      </div>
    </aside>
  )
}
