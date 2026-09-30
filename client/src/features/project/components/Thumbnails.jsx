import { assetUrl } from '../../../lib/api'

// Thumbnails shows every screen. The frame you are looking at is highlighted; the checkbox chooses it for generating code.
export default function Thumbnails({ screens, index, onIndex, selected, onToggle, onSelectAll, onClear }) {
  const all = selected.size === screens.length
  return (
    <section className="thumbs-block" aria-label="Screens">
      <div className="thumbs-head">
        <h2>
          Screens <span>({screens.length})</span>
        </h2>
        <div className="thumbs-actions">
          <span className="count" aria-live="polite">
            {selected.size} of {screens.length} selected
          </span>
          <button type="button" className="ghost" onClick={all ? onClear : onSelectAll}>
            {all ? 'Clear selection' : 'Select all'}
          </button>
        </div>
      </div>
      <div className="thumbnails">
        {screens.map((s, i) => {
          const on = selected.has(s.id)
          return (
            <div key={s.id} className={`thumb${i === index ? ' viewing' : ''}${on ? ' picked' : ''}`}>
              <button type="button" className="thumb-view" aria-pressed={i === index} aria-label={`View ${s.name}`} onClick={() => onIndex(i)}>
                {s.preview && s.preview.available ? <img src={assetUrl(s.preview.url)} alt="" loading="lazy" /> : <span className="thumb-empty" />}
                <small>{s.name}</small>
              </button>
              <label className="thumb-check">
                <input type="checkbox" checked={on} onChange={() => onToggle(s.id)} />
                <span className="sr-only">Use {s.name} for generating code</span>
              </label>
            </div>
          )
        })}
      </div>
    </section>
  )
}
