// A tiny store that counts running tasks, so one slim bar can show "something is loading".
let running = 0
const listeners = new Set()

const notify = () => listeners.forEach((fn) => fn())

// start marks a task as running and returns a function that marks it finished (safe to call twice).
export function startProgress() {
  let finished = false
  running += 1
  notify()
  return () => {
    if (finished) return
    finished = true
    running = Math.max(0, running - 1)
    notify()
  }
}

export const subscribeProgress = (fn) => {
  listeners.add(fn)
  return () => listeners.delete(fn)
}
export const progressCount = () => running
