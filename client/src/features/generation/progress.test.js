import test from 'node:test'
import assert from 'node:assert/strict'
import { doneCount, eventsFrom, stepsAfter } from './progress.js'

const steps = [
  { id: 'a', label: 'A', status: 'done', detail: 'first' },
  { id: 'b', label: 'B', status: 'done', detail: 'second' },
  { id: 'c', label: 'C', status: 'failed', detail: 'not yet' },
  { id: 'd', label: 'D', status: 'pending', detail: '' },
]

test('events follow the order things really happened', () => {
  assert.deepEqual(eventsFrom(steps).map((e) => `${e.i}:${e.to}`), ['0:running', '0:done', '1:running', '1:done', '2:running', '2:failed'])
})

test('a running step has only its start event', () => {
  assert.deepEqual(eventsFrom([{ status: 'running' }, { status: 'pending' }]), [{ i: 0, to: 'running' }])
})

test('showing fewer events shows fewer results, and details appear only when a step ends', () => {
  const ev = eventsFrom(steps)
  const early = stepsAfter(steps, ev, 1)
  assert.equal(early[0].status, 'running')
  assert.equal(early[0].detail, '')
  assert.equal(early[1].status, 'pending')
  const later = stepsAfter(steps, ev, 4)
  assert.deepEqual(later.map((s) => s.status), ['done', 'done', 'pending', 'pending'])
  assert.equal(later[1].detail, 'second')
  assert.equal(stepsAfter(steps, ev, ev.length)[2].status, 'failed')
})

test('the count only includes finished steps', () => {
  assert.equal(doneCount(stepsAfter(steps, eventsFrom(steps), 3)), 1)
  assert.equal(doneCount(stepsAfter(steps, eventsFrom(steps), 99)), 2)
})
