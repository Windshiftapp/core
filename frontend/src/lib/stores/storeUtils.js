/**
 * Utility helpers for Svelte writable stores.
 */

/**
 * Synchronously read the current value of a Svelte store.
 * @template T
 * @param {import('svelte/store').Readable<T>} store
 * @returns {T}
 */
export function getStoreValue(store) {
  let value;
  store.subscribe((v) => (value = v))();
  return value;
}

/**
 * Set every store in the list to null.
 * @param  {...import('svelte/store').Writable<any>} stores
 */
export function clearStores(...stores) {
  for (const store of stores) {
    store.set(null);
  }
}
