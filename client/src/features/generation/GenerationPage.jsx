import AppHeader from '../../components/AppHeader.jsx'
import Loader from '../../components/Loader.jsx'
import ProviderIcon from '../../components/ProviderIcon.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import Skeleton from '../../components/Skeleton.jsx'
import { Link } from '../../app/router.jsx'
import { projectPath } from '../../app/routes.js'
import { assetUrl } from '../../lib/api'
import { useProjectData } from '../project/hooks/useProjectData.js'
import StepList from './components/StepList.jsx'
import { useGenerationProgress } from './hooks/useGenerationProgress.js'
import { doneCount } from './progress.js'
import './generation.css'

export default function GenerationPage({ projectId, generationId, user, signOut }) {
  const { project, design } = useProjectData(projectId)
  const { generation, steps, finished, error, retry } = useGenerationProgress(projectId, generationId)
  const back = projectPath(projectId)

  const total = steps.length || 1
  const percent = Math.round((doneCount(steps) / total) * 100)
  const first = design && design.screens && design.screens[0]
  const image = first && first.preview && first.preview.available ? assetUrl(first.preview.url) : ''

  let heading = 'Getting started'
  if (generation) {
    if (!finished) heading = `Generating code with ${generation.provider_label}`
    else if (generation.status === 'completed') heading = 'Your code is ready'
    else heading = 'Generation stopped'
  }

  return (
    <div className="page generation-page">
      <AppHeader user={user} onSignOut={signOut} />
      <main className="generation-main">
        <nav className="crumbs" aria-label="Breadcrumb">
          <Link to="/">Projects</Link>
          <span aria-hidden="true">›</span>
          <Link to={back}>{project ? project.name : 'Project'}</Link>
          <span aria-hidden="true">›</span>
          <span>Generate</span>
        </nav>

        {error ? (
          <div className="gen-message">
            <h1>{error === 'missing' ? 'We can’t find that generation' : 'We lost contact with Layr'}</h1>
            <p>{error === 'missing' ? 'It may belong to another project, or the link is wrong.' : error}</p>
            <div className="row">
              {error !== 'missing' && (
                <button type="button" className="primary" onClick={retry}>
                  Try again
                </button>
              )}
              <Link className="secondary" to={back}>
                Back to the project
              </Link>
            </div>
          </div>
        ) : (
          <div className="gen-grid">
            <section className="gen-design" aria-label="Your design">
              <div className="gen-image">{image ? <img src={image} alt="" /> : <Skeleton style={{ width: '100%', height: '100%', borderRadius: 12 }} />}</div>
              <h2>{project ? project.name : <Skeleton className="pill" style={{ width: 160, height: 18 }} />}</h2>
              {design && design.counts && (
                <p className="gen-meta">
                  {design.counts.screens} {design.counts.screens === 1 ? 'screen' : 'screens'}
                  {design.source && design.source.file_name ? ` from ${design.source.file_name}` : ''}
                  {generation && generation.screen_ids ? `. Using ${generation.screen_ids.length} of them.` : generation ? '. Using all of them.' : ''}
                </p>
              )}
            </section>

            <section className="gen-progress" aria-label="Progress" aria-live="polite">
              {!generation ? (
                <div className="gen-loading">
                  <Loader size={44} label="Loading progress" />
                </div>
              ) : (
                <>
                  <div className="gen-head">
                    <ProviderIcon id={generation.provider} size={40} />
                    <div>
                      <h1>{heading}</h1>
                      <p>{finished ? (generation.status === 'completed' ? 'Everything finished.' : 'Here is what happened.') : 'You can leave this page open while it runs.'}</p>
                    </div>
                  </div>

                  <div className="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={percent} aria-label="Progress">
                    <span style={{ width: `${percent}%` }} />
                  </div>
                  <p className="bar-note">
                    {doneCount(steps)} of {steps.length} steps done
                  </p>

                  <StepList steps={steps} />

                  {finished && generation.error && (
                    <div className={generation.error.code === 'GENERATION_NOT_AVAILABLE' ? 'result info' : 'result error'} role="status">
                      <p>{generation.error.message}</p>
                    </div>
                  )}
                  {finished && (
                    <div className="row">
                      <Link className="primary" to={back}>
                        Back to the preview
                      </Link>
                    </div>
                  )}
                </>
              )}
            </section>
          </div>
        )}
      </main>
      <SiteFooter withBrand />
    </div>
  )
}
