import { useEffect, useMemo, useRef, useState } from 'react'

// SelectScreens lets the person choose which frames of a multi-screen file to import.
export default function SelectScreens({ frames, fileName, onConfirm, onCancel }) {
  const dialog = useRef(null)
  const [chosen, setChosen] = useState(() => new Set())

  useEffect(() => {
    dialog.current.showModal()
  }, [])

  const pages = useMemo(() => {
    const groups = new Map()
    for (const f of frames) groups.set(f.page || 'Page', [...(groups.get(f.page || 'Page') || []), f])
    return [...groups]
  }, [frames])

  const toggle = (id) =>
    setChosen((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  const all = chosen.size === frames.length
  const confirm = () => onConfirm(all ? { all: true } : { node_ids: [...chosen] })

  return (
    <dialog ref={dialog} onCancel={onCancel} aria-labelledby="select-title">
      <div className="dialog-heading">
        <h2 id="select-title">Choose screens to import</h2>
      </div>
      <p className="dialog-note">{fileName ? `“${fileName}” has ${frames.length} screens.` : `This file has ${frames.length} screens.`} Pick the ones you want.</p>

      <div className="select-actions">
        <button type="button" className="ghost" onClick={() => setChosen(all ? new Set() : new Set(frames.map((f) => f.id)))}>
          {all ? 'Clear selection' : 'Select all'}
        </button>
        <span>{chosen.size} selected</span>
      </div>

      <div className="frame-list">
        {pages.map(([page, items]) => (
          <fieldset key={page}>
            <legend>{page}</legend>
            {items.map((f) => (
              <label key={f.id}>
                <input type="checkbox" checked={chosen.has(f.id)} onChange={() => toggle(f.id)} />
                <span>{f.name}</span>
                {f.type === 'SECTION' && <em>section</em>}
              </label>
            ))}
          </fieldset>
        ))}
      </div>

      <div className="dialog-footer">
        <button type="button" className="ghost" onClick={onCancel}>
          Cancel
        </button>
        <button type="button" className="primary" disabled={chosen.size === 0} onClick={confirm}>
          Import {chosen.size || ''} {chosen.size === 1 ? 'screen' : 'screens'}
        </button>
      </div>
    </dialog>
  )
}
