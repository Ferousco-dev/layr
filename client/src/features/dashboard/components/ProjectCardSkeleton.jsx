import Skeleton from '../../../components/Skeleton.jsx'

// ProjectCardSkeleton mirrors ProjectCard so nothing jumps when real projects arrive.
export default function ProjectCardSkeleton() {
  return (
    <article className="card" aria-hidden="true">
      <div className="preview">
        <Skeleton style={{ width: '100%', height: '100%', borderRadius: 0 }} />
      </div>
      <div className="card-heading">
        <Skeleton className="pill" style={{ width: '55%', height: 16 }} />
      </div>
      <div className="metadata">
        <Skeleton className="round" style={{ width: 24, height: 24 }} />
        <Skeleton className="pill" style={{ width: '35%', height: 12 }} />
        <Skeleton className="pill" style={{ width: 52, height: 10, marginLeft: 'auto' }} />
      </div>
    </article>
  )
}
