import { useEffect, useRef, useState } from 'react'
import ProviderIcon from '../../../components/ProviderIcon.jsx'
import { generationLabel } from '../../../lib/providers.js'
import { lastProvider } from '../hooks/useGenerateFlow.js'

// ChooseModelDialog appears when more than one AI key is saved.
export default function ChooseModelDialog({ keys, onChoose, onCancel }) {
  const dialog = useRef(null)
  const remembered = lastProvider()
  const [provider, setProvider] = useState(keys.some((k) => k.provider === remembered) ? remembered : keys[0].provider)

  useEffect(() => {
    dialog.current.showModal()
  }, [])

  return (
    <dialog ref={dialog} className="flow-dialog" onClose={onCancel} aria-labelledby="choose-title">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          onChoose(provider)
        }}
      >
        <h2 id="choose-title">Which model should write the code?</h2>
        <p className="lead">You have more than one key saved. Pick the one to use for this project.</p>
        <fieldset className="choices">
          <legend className="sr-only">AI model</legend>
          {keys.map((k) => (
            <label key={k.provider} className={provider === k.provider ? 'choice on' : 'choice'}>
              <input type="radio" name="model" value={k.provider} checked={provider === k.provider} onChange={() => setProvider(k.provider)} />
              <ProviderIcon id={k.provider} size={34} />
              <span>
                <b>{generationLabel(k.provider)}</b>
                <small>Key ending in {k.hint}</small>
              </span>
            </label>
          ))}
        </fieldset>
        <div className="dialog-actions">
          <button type="button" className="secondary" onClick={() => dialog.current.close()}>
            Cancel
          </button>
          <button type="submit" className="primary">
            Generate with this model
          </button>
        </div>
      </form>
    </dialog>
  )
}
