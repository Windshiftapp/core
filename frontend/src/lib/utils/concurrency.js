// Bounded concurrency pool for uploads and other per-item request batches.
//
// Attachment uploads used to run either one file per request in series (a long
// waterfall for a large drop) or all at once (a burst that competes with
// foreground reads for the server's per-user concurrency budget). Every path
// now funnels through `runWithConcurrency` with one small limit.

export const UPLOAD_CONCURRENCY = 3;

/**
 * Run `worker` over `items` with at most `concurrency` calls in flight.
 *
 * Settles every item: a rejected worker is recorded, never thrown, so one
 * failing file cannot cancel the rest of the batch. Callers inspect each
 * result and decide how to report failures.
 *
 * @template T, R
 * @param {Iterable<T>} items
 * @param {(item: T, index: number) => Promise<R>} worker
 * @param {{ concurrency?: number }} [options]
 * @returns {Promise<Array<{ item: T, index: number, ok: boolean, value: R | null, error: any }>>}
 *   settled results in input order
 */
export async function runWithConcurrency(items, worker, { concurrency = UPLOAD_CONCURRENCY } = {}) {
  const list = Array.from(items);
  /** @type {Array<{ item: T, index: number, ok: boolean, value: R | null, error: any }>} */
  const results = new Array(list.length);
  const limit = Math.max(1, Math.min(Math.floor(concurrency) || 1, list.length || 1));
  let cursor = 0;

  async function runWorker() {
    while (cursor < list.length) {
      const index = cursor++;
      const item = list[index];
      try {
        results[index] = { item, index, ok: true, value: await worker(item, index), error: null };
      } catch (error) {
        results[index] = { item, index, ok: false, value: null, error };
      }
    }
  }

  await Promise.all(Array.from({ length: limit }, runWorker));
  return results;
}
