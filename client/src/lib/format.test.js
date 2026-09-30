import test from 'node:test'
import assert from 'node:assert/strict'
import { asFigmaUrl, projectNameFromUrl, timeAgo } from './format.js'

const NOW = Date.parse('2026-09-30T12:00:00Z')

test('timeAgo picks the largest whole unit', () => {
  assert.equal(timeAgo('2026-09-30T11:59:40Z', NOW), 'just now')
  assert.equal(timeAgo('2026-09-30T11:59:00Z', NOW), '1 minute ago')
  assert.equal(timeAgo('2026-09-30T10:00:00Z', NOW), '2 hours ago')
  assert.equal(timeAgo('2026-09-28T12:00:00Z', NOW), '2 days ago')
  assert.equal(timeAgo('2026-09-16T12:00:00Z', NOW), '2 weeks ago')
  assert.equal(timeAgo('2026-01-30T12:00:00Z', NOW), '8 months ago')
})

test('timeAgo never reports the future or garbage', () => {
  assert.equal(timeAgo('2026-10-01T12:00:00Z', NOW), 'just now')
  assert.equal(timeAgo('not a date', NOW), '')
})

test('asFigmaUrl accepts only figma.com links', () => {
  assert.ok(asFigmaUrl('https://www.figma.com/design/AbC123/My-File?node-id=1-2'))
  assert.ok(asFigmaUrl('figma.com/design/AbC123/My-File'))
  assert.equal(asFigmaUrl('https://figma.com.evil.example/design/AbC123'), '')
  assert.equal(asFigmaUrl('https://evil.example/?u=figma.com'), '')
  assert.equal(asFigmaUrl('landing page'), '')
  assert.equal(asFigmaUrl(''), '')
  assert.equal(asFigmaUrl('javascript:alert(1)'), '')
})

test('projectNameFromUrl reads the file slug', () => {
  assert.equal(projectNameFromUrl('https://www.figma.com/design/AbC123/Landing-Page?x=1'), 'Landing Page')
  assert.equal(projectNameFromUrl('https://www.figma.com/design/AbC123'), 'Untitled design')
  assert.equal(projectNameFromUrl('nonsense'), 'Untitled design')
  assert.ok(projectNameFromUrl('https://www.figma.com/design/AbC123/' + 'a'.repeat(300)).length <= 120)
})
