import Loader from '../../../components/Loader.jsx'
import { assetUrl } from '../../../lib/api'
import { useViewer } from '../hooks/useViewer.js'

export default function Viewer({ screens, index, onIndex, refresh }) {
  const screen = screens[index]
  const v = useViewer(screen.id)
  const image = screen.preview && screen.preview.available ? assetUrl(screen.preview.url) : ''
  const step = (n) => onIndex((index + n + screens.length) % screens.length)

  return (
    <section className="viewer-card" aria-label="Design preview">
      <div className="viewer" ref={v.box}>
        <div className="viewer-top">
          <div className="pager">
            <button type="button" aria-label="Previous screen" onClick={() => step(-1)} disabled={screens.length < 2}>
              ‹
            </button>
            <span>
              {index + 1} / {screens.length}
            </span>
            <button type="button" className="refresh" aria-label="Refresh from Figma" title="Refresh from Figma: import this design again" onClick={refresh.run} disabled={refresh.state.phase === 'working'}>
              {refresh.state.phase === 'working' ? <Loader size={20} /> : '↻'}
            </button>
            <button type="button" aria-label="Next screen" onClick={() => step(1)} disabled={screens.length < 2}>
              ›
            </button>
          </div>
          <div className="zoom-tools">
            <div className="zoom">
              <button type="button" aria-label="Zoom out" onClick={v.zoomOut} disabled={!v.canZoomOut}>
                −
              </button>
              <button type="button" className="zoom-value" aria-label="Reset zoom and position" title="Reset view" onClick={v.reset}>
                {Math.round(v.view.zoom * 100)}%
              </button>
              <button type="button" aria-label="Zoom in" onClick={v.zoomIn} disabled={!v.canZoomIn}>
                +
              </button>
            </div>
            <button type="button" className="fs" aria-label={v.fullscreen ? 'Exit fullscreen' : 'Enter fullscreen'} onClick={v.toggleFullscreen}>
              ⛶
            </button>
          </div>
        </div>
        <div className="canvas" tabIndex={0} aria-label="Design canvas. Drag to pan; use plus or minus to zoom." {...v.handlers}>
          {image ? (
            <img src={image} alt={`${screen.name} design`} draggable="false" style={{ transform: `translate(${v.view.x}px, ${v.view.y}px) scale(${v.view.zoom})` }} />
          ) : (
            <p className="no-image">No preview is available for this screen.</p>
          )}
        </div>
      </div>
      {refresh.state.message && (
        <p className={refresh.state.phase === 'error' ? 'refresh-note error' : 'refresh-note'} role="status">
          {refresh.state.message}
          {refresh.state.phase === 'error' && (
            <button type="button" className="link" onClick={refresh.dismiss}>
              Dismiss
            </button>
          )}
        </p>
      )}
    </section>
  )
}
