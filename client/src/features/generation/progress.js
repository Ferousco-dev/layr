// Turns the steps the server reports into a sequence of events that can be shown one at a time.
// Nothing is invented: every event is a state change the server has already recorded.

// eventsFrom lists what has happened so far: a step started, then finished or failed.
export function eventsFrom(steps) {
  const events = []
  steps.forEach((s, i) => {
    if (s.status !== 'pending') events.push({ i, to: 'running' })
    if (s.status === 'done' || s.status === 'failed') events.push({ i, to: s.status })
  })
  return events
}

// stepsAfter shows the steps as they looked after the first `count` events.
export function stepsAfter(steps, events, count) {
  const out = steps.map((s) => ({ id: s.id, label: s.label, status: 'pending', detail: '' }))
  events.slice(0, count).forEach((e) => {
    out[e.i].status = e.to
    if (e.to !== 'running') out[e.i].detail = steps[e.i].detail || ''
  })
  return out
}

export const doneCount = (steps) => steps.filter((s) => s.status === 'done').length
