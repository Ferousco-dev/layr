import Skeleton from '../../../components/Skeleton.jsx'

// ProjectSkeleton holds the preview page's shape while the design loads.
export default function ProjectSkeleton() {
  return (
    <div className="project-grid" aria-busy="true" aria-label="Loading the project">
      <div className="left-column">
        <div className="viewer-card">
          <Skeleton style={{ width: '100%', height: 468, borderRadius: 12 }} />
        </div>
        <div className="thumbnails">
          {[0, 1, 2, 3, 4].map((i) => (
            <Skeleton key={i} style={{ width: 150, height: 90, borderRadius: 10, flex: 'none' }} />
          ))}
        </div>
      </div>
      <aside className="details-card">
        <div className="project-heading">
          <Skeleton style={{ width: 112, height: 112, borderRadius: 10, flex: 'none' }} />
          <div className="skeleton-col">
            <Skeleton className="pill" style={{ width: '70%', height: 24 }} />
            <Skeleton className="pill" style={{ width: '50%', height: 14 }} />
          </div>
        </div>
        <div className="skeleton-col spaced">
          {[0, 1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="pill" style={{ width: '100%', height: 16 }} />
          ))}
        </div>
      </aside>
    </div>
  )
}
