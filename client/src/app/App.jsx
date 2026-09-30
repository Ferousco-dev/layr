import { lazy, Suspense, useEffect, useState } from 'react'
import ContactPage from '../features/contact/ContactPage.jsx'
import Landing from '../features/landing/Landing.jsx'
import LegalPage from '../features/legal/LegalPage.jsx'
import { privacy } from '../features/legal/content/privacy.js'
import { terms } from '../features/legal/content/terms.js'
import NotFoundPage from '../features/notfound/NotFoundPage.jsx'
import InfoPage from '../features/pages/InfoPage.jsx'
import AppShellSkeleton from '../components/AppShellSkeleton.jsx'
import { isKnownRoute, matchDynamic, normalizePath } from './routes.js'
import { finishNavigation, usePath } from './router.jsx'
import { usePageMeta } from './usePageMeta.js'
import { useSession, wasSignedIn } from './useSession.js'

const loadDashboard = () => import('../features/dashboard/Dashboard.jsx')
const loadProfile = () => import('../features/profile/Profile.jsx')
const loadProject = () => import('../features/project/ProjectPage.jsx')
const loadGeneration = () => import('../features/generation/GenerationPage.jsx')
const Dashboard = lazy(loadDashboard)
const Profile = lazy(loadProfile)
const ProjectPage = lazy(loadProject)
const GenerationPage = lazy(loadGeneration)

const INFO_PAGES = {
  '/docs': 'Docs',
}

const LEGAL = {
  '/privacy': { doc: privacy, description: 'How Layr collects, uses, protects and deletes your data.' },
  '/terms': { doc: terms, description: 'The terms that apply when you use Layr.' },
}

const HOME = {
  title: 'Layr | Turn your Figma designs into real code',
  description: 'Connect your Figma account and turn your designs into production-ready code with Layr.',
}

export default function App() {
  const path = normalizePath(usePath())
  const known = isKnownRoute(path)
  const [returning] = useState(wasSignedIn)
  const infoTitle = INFO_PAGES[path]
  const legal = LEGAL[path]
  const contact = path === '/contact'
  const dynamic = matchDynamic(path)
  const session = useSession()
  const signedIn = session.status === 'signed-in'

  useEffect(() => {
    finishNavigation()
  }, [path])

  useEffect(() => {
    if (signedIn) {
      loadDashboard()
      loadProfile()
      loadProject()
    }
  }, [signedIn])

  usePageMeta(
    !known
      ? { title: 'Page not found | Layr', description: HOME.description, path: '/', indexable: false }
      : contact
      ? { title: 'Contact | Layr', description: 'Get in touch with the Layr team.', path, indexable: true }
      : legal
      ? { title: `${legal.doc.title} | Layr`, description: legal.description, path, indexable: true }
      : infoTitle
      ? { title: `${infoTitle} | Layr`, description: HOME.description, path, indexable: false }
      : signedIn && dynamic
        ? { title: dynamic.name === 'generation' ? 'Generating code | Layr' : 'Project | Layr', description: HOME.description, path, indexable: false }
        : signedIn && path === '/profile'
        ? { title: 'Profile | Layr', description: HOME.description, path, indexable: false }
        : signedIn
          ? { title: 'Your projects | Layr', description: HOME.description, path: '/', indexable: false }
          : { ...HOME, path: '/', indexable: true },
  )

  if (!known) return <NotFoundPage />
  if (contact) return <ContactPage />
  if (legal) return <LegalPage doc={legal.doc} />
  if (infoTitle) return <InfoPage title={infoTitle} />

  const shell = dynamic ? 'project' : path === '/profile' ? 'profile' : 'projects'

  if (signedIn) {
    const common = { user: session.user, signOut: session.signOut }
    let page = <Dashboard {...common} />
    if (dynamic && dynamic.name === 'generation') page = <GenerationPage {...common} key={dynamic.generationId} projectId={dynamic.projectId} generationId={dynamic.generationId} />
    else if (dynamic) page = <ProjectPage {...common} key={dynamic.projectId} projectId={dynamic.projectId} />
    else if (path === '/profile') page = <Profile {...common} />
    return <Suspense fallback={<AppShellSkeleton kind={shell} />}>{page}</Suspense>
  }

  if (session.status === 'checking' && returning) return <AppShellSkeleton kind={shell} />

  return <Landing checking={session.status === 'checking'} unreachable={session.unreachable} />
}
