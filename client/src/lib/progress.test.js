import test from 'node:test'
import assert from 'node:assert/strict'
import { progressCount, startProgress, subscribeProgress } from './progress.js'

test('the count follows running tasks', () => {
  const a = startProgress()
  const b = startProgress()
  assert.equal(progressCount(), 2)
  a()
  assert.equal(progressCount(), 1)
  b()
  assert.equal(progressCount(), 0)
})

test('finishing twice does not undercount', () => {
  const a = startProgress()
  const b = startProgress()
  a()
  a()
  assert.equal(progressCount(), 1)
  b()
  assert.equal(progressCount(), 0)
})

test('listeners hear about every change', () => {
  let calls = 0
  const off = subscribeProgress(() => calls++)
  const done = startProgress()
  done()
  off()
  startProgress()()
  assert.equal(calls, 2)
})
