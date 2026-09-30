import { Link } from '../../../app/router.jsx'
import { projectPath } from '../../../app/routes.js'
import { assetUrl } from '../../../lib/api'
import { initials, timeAgo } from '../../../lib/format.js'
import Skeleton from '../../../components/Skeleton.jsx'
import { useDesign } from '../hooks/useDesign.js'

export function firstPreview(design) {
  const screen = design && design.screens && design.screens.find((s) => s.preview && s.preview.available)
  return screen ? assetUrl(screen.preview.url) : ''
}

export default function ProjectCard({ project, user, version }) {
  const { ref, design } = useDesign(project.id, version)
  const image = firstPreview(design)

  return (
    <article className="card" ref={ref}>
      <Link className="preview" to={projectPath(project.id)} aria-label={`Open ${project.name}`}>
        {image ? (
          <img src={image} alt={`${project.name} design preview`} loading="lazy" />
        ) : (
          design === undefined ? <Skeleton style={{ width: '100%', height: '100%', borderRadius: 0 }} /> : <span className="no-preview">No preview yet</span>
        )}
      </Link>
      <div className="card-heading">
        <Link className="project-title" to={projectPath(project.id)}>
          {project.name}
        </Link>
      </div>
      <div className="metadata">
        {user.avatar_url ? <img src={user.avatar_url} alt="" referrerPolicy="no-referrer" /> : <span className="avatar-fallback">{initials(user.name)}</span>}
        <span>{user.name}</span>
        <time dateTime={project.updated_at}>{timeAgo(project.updated_at)}</time>
      </div>
    </article>
  )
}
