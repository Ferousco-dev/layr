import { useCallback, useRef, useState } from 'react'
import { ApiError, createProject, deleteProject, latestImport, renameProject, selectFrames, startImport } from '../../../lib/api'
import { projectNameFromUrl } from '../../../lib/format.js'
import { MAX_ALL_SCREENS } from '../../../lib/limits.js'

const POLL_MS = 1500
const MAX_POLLS = 200
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

// useImporter runs the whole "paste a Figma link" flow: project, import, optional screen choice, rename.
export function useImporter(onImported) {
  const [state, setState] = useState({ phase: 'idle', message: '' })
  const current = useRef(null)

  const discard = useCallback(async () => {
    const project = current.current
    current.current = null
    if (project) await deleteProject(project.id).catch(() => {})
  }, [])

  const fail = useCallback(
    async (err) => {
      await discard()
      setState({ phase: 'error', message: err instanceof ApiError ? err.message : 'The import did not finish. Please try again.' })
    },
    [discard],
  )

  const follow = useCallback(
    async (project) => {
      for (let i = 0; i < MAX_POLLS; i++) {
        await sleep(POLL_MS)
        if (current.current !== project) return
        const imp = await latestImport(project.id)
        if (imp.status === 'awaiting_selection' && (imp.frames || []).length <= MAX_ALL_SCREENS) {
          setState({ phase: 'working', message: `Importing all ${(imp.frames || []).length} screens…` })
          await selectFrames(project.id, imp.id, { all: true })
          continue
        }
        if (imp.status === 'awaiting_selection') {
          setState({ phase: 'selecting', message: '', frames: imp.frames || [], importId: imp.id, fileName: imp.figma_file_name })
          return
        }
        if (imp.status === 'completed') {
          const name = (imp.figma_file_name || '').slice(0, 120)
          if (name && name !== project.name) await renameProject(project.id, name).catch(() => {})
          current.current = null
          setState({ phase: 'idle', message: '' })
          onImported(project.id)
          return
        }
        if (imp.status === 'failed') {
          throw new ApiError(0, (imp.error && imp.error.code) || 'IMPORT_FAILED', (imp.error && imp.error.message) || 'The import failed.')
        }
        setState({ phase: 'working', message: imp.status === 'pending' ? 'Starting the import…' : 'Importing your design…' })
      }
      throw new ApiError(0, 'IMPORT_TIMEOUT', 'The import is taking too long. Please try again.')
    },
    [onImported],
  )

  const start = useCallback(
    async (figmaUrl) => {
      await discard()
      try {
        setState({ phase: 'working', message: 'Creating your project…' })
        const project = await createProject(projectNameFromUrl(figmaUrl))
        current.current = project
        setState({ phase: 'working', message: 'Reading your Figma file…' })
        await startImport(project.id, figmaUrl)
        await follow(project)
      } catch (err) {
        await fail(err)
      }
    },
    [discard, follow, fail],
  )

  const choose = useCallback(
    async (selection) => {
      const project = current.current
      const importId = state.importId
      if (!project) return
      try {
        setState({ phase: 'working', message: 'Importing the screens you chose…' })
        await selectFrames(project.id, importId, selection)
        await follow(project)
      } catch (err) {
        await fail(err)
      }
    },
    [follow, fail, state.importId],
  )

  const cancel = useCallback(async () => {
    await discard()
    setState({ phase: 'idle', message: '' })
  }, [discard])

  const dismiss = useCallback(() => setState({ phase: 'idle', message: '' }), [])

  return { state, start, choose, cancel, dismiss }
}
