// A small in-memory store so a page you have already opened shows its last data at once, then refreshes quietly.
const store = new Map()

export const remember = (key, value) => store.set(key, value)
export const recall = (key) => store.get(key)
export const forget = (key) => store.delete(key)
export const forgetAll = () => store.clear()
