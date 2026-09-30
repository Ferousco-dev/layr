import Loader from '../../../components/Loader.jsx'

const Mark = ({ status }) => {
  if (status === 'running') return <Loader size={22} />
  if (status === 'done')
    return (
      <span className="mark done" aria-hidden="true">
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
          <path d="m5 12 5 5 9-10" />
        </svg>
      </span>
    )
  if (status === 'failed')
    return (
      <span className="mark failed" aria-hidden="true">
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round">
          <path d="M7 7l10 10M17 7 7 17" />
        </svg>
      </span>
    )
  return <span className="mark pending" aria-hidden="true" />
}

const WORDS = { pending: 'Waiting', running: 'In progress', done: 'Done', failed: 'Stopped' }

export default function StepList({ steps }) {
  return (
    <ol className="steps">
      {steps.map((s) => (
        <li key={s.id} className={`step ${s.status}`}>
          <Mark status={s.status} />
          <div>
            <p className="step-label">
              {s.label}
              <span className="sr-only"> ({WORDS[s.status]})</span>
            </p>
            {s.detail && <p className="step-detail">{s.detail}</p>}
          </div>
        </li>
      ))}
    </ol>
  )
}
