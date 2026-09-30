import { useEffect, useState } from 'react'
import Loader from '../../../components/Loader.jsx'
import { assetUrl, loadAssets } from '../../../lib/api'

const size = (bytes) => (bytes >= 1048576 ? `${(bytes / 1048576).toFixed(1)} MB` : `${Math.max(1, Math.round(bytes / 1024))} KB`)

// AssetsTab shows every image and vector Figma gave us, right in the page.
export default function AssetsTab({ projectId, screens }) {
  const [assets, setAssets] = useState(undefined)
  const [failed, setFailed] = useState(false)
  const [kind, setKind] = useState('all')
  const names = Object.fromEntries(screens.map((s) => [s.id, s.name]))

  useEffect(() => {
    let active = true
    setFailed(false)
    loadAssets(projectId)
      .then((a) => active && setAssets(a))
      .catch(() => active && setFailed(true))
    return () => {
      active = false
    }
  }, [projectId])

  if (failed) return <p className="tab-note">Could not load the assets. Please try again.</p>
  if (!assets)
    return (
      <div className="tab-loading">
        <Loader size={28} label="Loading assets" />
      </div>
    )
  if (!assets.length) return <p className="tab-note">Figma did not provide any images or icons for this design.</p>

  const images = assets.filter((a) => a.kind === 'image').length
  const shown = assets.filter((a) => kind === 'all' || (kind === 'image' ? a.kind === 'image' : a.kind !== 'image'))

  return (
    <div className="tab-body assets">
      <div className="asset-filter" role="group" aria-label="Filter assets">
        {[
          ['all', `All (${assets.length})`],
          ['image', `Images (${images})`],
          ['vector', `Icons and vectors (${assets.length - images})`],
        ].map(([id, label]) => (
          <button key={id} type="button" className={kind === id ? 'on' : ''} aria-pressed={kind === id} onClick={() => setKind(id)}>
            {label}
          </button>
        ))}
      </div>
      <ul className="asset-grid">
        {shown.map((a) => (
          <li key={a.id}>
            <div className="asset-thumb">
              <img src={assetUrl(a.url)} alt={a.name} loading="lazy" />
            </div>
            <b title={a.name}>{a.name}</b>
            <small>
              {a.format.toUpperCase()}
              {a.width ? ` · ${Math.round(a.width)}×${Math.round(a.height)}` : ''} · {size(a.size_bytes)}
            </small>
            <small>{a.screen_ids.length ? `On ${a.screen_ids.length === 1 ? names[a.screen_ids[0]] || '1 screen' : `${a.screen_ids.length} screens`}` : 'Not placed on a screen'}</small>
          </li>
        ))}
      </ul>
    </div>
  )
}
