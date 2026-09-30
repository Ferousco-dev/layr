import Brand from './Brand.jsx'
import Skeleton from './Skeleton.jsx'
import './shell.css'

const bar = (width, height = 14) => <Skeleton className="pill" style={{ width, height }} />

// AppShellSkeleton stands in for a signed-in page while it loads, so nothing else flashes on screen.
export default function AppShellSkeleton({ kind = 'projects' }) {
  return (
    <div className="shell" aria-busy="true" aria-label="Loading">
      <header className="shell-header">
        <Brand />
        <Skeleton className="pill" style={{ width: 168, height: 50 }} />
      </header>
      {kind === 'project' ? (
        <main className="shell-main">
          <div className="shell-project">
            <div className="shell-stack">
              <Skeleton style={{ width: '100%', height: 468, borderRadius: 16 }} />
              <Skeleton className="pill" style={{ width: '60%', height: 60 }} />
            </div>
            <div className="shell-panel single">
              {bar('70%', 24)}
              {bar('100%')}
              {bar('100%')}
              {bar('100%', 56)}
            </div>
          </div>
        </main>
      ) : kind === 'profile' ? (
        <main className="shell-main narrow">
          {[0, 1, 2].map((i) => (
            <section className="shell-panel" key={i}>
              <div className="shell-panel-head">
                {bar(120, 20)}
                {bar('85%')}
              </div>
              <div className="shell-stack">
                {bar('100%', 44)}
                {bar('100%', 44)}
              </div>
            </section>
          ))}
        </main>
      ) : (
        <main className="shell-main">
          <div className="shell-hero">
            {bar('min(420px, 70%)', 34)}
            {bar('min(320px, 60%)', 16)}
            {bar('min(760px, 100%)', 64)}
          </div>
          <div className="shell-grid">
            {Array.from({ length: 8 }, (_, i) => (
              <div className="shell-card" key={i}>
                <Skeleton style={{ width: '100%', aspectRatio: '2.15', borderRadius: 12 }} />
                {bar('55%', 16)}
                {bar('80%', 12)}
              </div>
            ))}
          </div>
        </main>
      )}
    </div>
  )
}
