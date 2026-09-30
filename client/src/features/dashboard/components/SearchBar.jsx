import Loader from '../../../components/Loader.jsx'

// SearchBar is both the project filter and the "paste a Figma link" import box.
export default function SearchBar({ query, onChange, onSubmit, busy, looksLikeLink, importState, onDismiss }) {
  return (
    <>
      <form className="search" onSubmit={onSubmit}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" aria-hidden="true">
          <circle cx="10.5" cy="10.5" r="7.5" />
          <path d="m16 16 5 5" />
        </svg>
        <label className="sr-only" htmlFor="search">
          Search projects or paste a Figma URL
        </label>
        <input id="search" value={query} onChange={(e) => onChange(e.target.value)} disabled={busy} placeholder="Search your projects (e.g. Landing Page) or paste a Figma URL..." autoComplete="off" />
        <button type="submit" disabled={busy} aria-label={looksLikeLink ? 'Import this Figma design' : 'Search projects'}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" aria-hidden="true">
            <path d="M3 12h18m-7-7 7 7-7 7" />
          </svg>
        </button>
      </form>
      <p id="notice" role="status" className={importState.phase === 'error' ? 'notice error' : 'notice'}>
        {importState.phase === 'working' && <Loader size={26} />}
        {importState.message}
        {importState.phase === 'error' && (
          <button type="button" className="link" onClick={onDismiss}>
            Dismiss
          </button>
        )}
      </p>
    </>
  )
}
