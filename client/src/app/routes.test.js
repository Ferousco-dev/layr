import test from 'node:test'
import assert from 'node:assert/strict'
import { isKnownRoute, normalizePath } from './routes.js'

test('known pages are found, with or without a trailing slash', () => {
  for (const p of ['/', '/profile', '/profile/', '/terms', '/privacy', '/contact', '/docs']) assert.ok(isKnownRoute(p), p)
})

test('anything else is not', () => {
  for (const p of ['/nope', '/profile/extra', '/api/v1/me', '/index.html', '//x']) assert.ok(!isKnownRoute(p), p)
})

test('normalizePath trims slashes', () => {
  assert.equal(normalizePath('/terms//'), '/terms')
  assert.equal(normalizePath(''), '/')
})

test('pages with ids are recognised and give their parts', async () => {
  const { matchDynamic, generationPath, projectPath } = await import('./routes.js')
  const p = '11111111-2222-4333-8444-555555555555'
  const g = 'aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee'
  assert.deepEqual(matchDynamic(projectPath(p)), { name: 'project', projectId: p })
  assert.deepEqual(matchDynamic(generationPath(p, g) + '/'), { name: 'generation', projectId: p, generationId: g })
  assert.ok(isKnownRoute(projectPath(p)) && isKnownRoute(generationPath(p, g)))
})

test('ids must be real ids', () => {
  for (const bad of ['/projects/abc', '/projects/../etc', '/projects/11111111-2222-4333-8444-55555555555', '/projects/11111111-2222-4333-8444-555555555555/x', '/projects']) {
    assert.ok(!isKnownRoute(bad), bad)
  }
})
