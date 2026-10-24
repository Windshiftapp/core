// Shared per-tab read scheduler for opted-in API requests.
//
// GH #338: several independent hydration families (dependencies, assignee
// hydration, SLA chunks, follow-up board pages) each launched their own
// parallel fetches. A single per-tab budget keeps the combined fan-out below
// the server's per-user concurrency limit instead of summing component-local
// pools. The server limiter stays authoritative: these budgets are a chosen
// starting point, not a guarantee against other tabs or deployments.
//
// A slot covers the whole physical attempt, including response-body
// consumption, and is released exactly once for every outcome.

export const READ_PRIORITY = Object.freeze({
  FOREGROUND: 'foreground',
  BACKGROUND: 'background',
});

// Reusable request-option presets for the two scheduler classes.
export const FOREGROUND_READ = Object.freeze({
  readPolicy: { priority: READ_PRIORITY.FOREGROUND },
});
export const BACKGROUND_READ = Object.freeze({
  readPolicy: { priority: READ_PRIORITY.BACKGROUND },
});

export const MAX_ACTIVE_READS = 8;
export const MAX_ACTIVE_BACKGROUND_READS = 2;
export const MAX_PENDING_READS = 256;
// Dispatch three foreground attempts for every background attempt while both
// classes are ready, so background hydration cannot starve foreground loads
// (and vice versa) when a queue backlog builds up.
export const FOREGROUND_BURST = 3;

/** @type {any[]} */
const foregroundQueue = [];
/** @type {any[]} */
const backgroundQueue = [];
const activeJobs = new Set();
let activeBackgroundCount = 0;
let foregroundSinceBackground = 0;

function abortError() {
  return new DOMException('The read was aborted', 'AbortError');
}

function queueFullError() {
  const error = new Error('Too many read requests are queued. Please try again shortly.');
  /** @type {any} */ (error).status = 0;
  /** @type {any} */ (error).code = 'REQUEST_QUEUE_FULL';
  return error;
}

function queueFor(priority) {
  return priority === READ_PRIORITY.BACKGROUND ? backgroundQueue : foregroundQueue;
}

function removeFromQueue(job) {
  const queue = queueFor(job.priority);
  const index = queue.indexOf(job);
  if (index !== -1) queue.splice(index, 1);
}

function cleanupJob(job) {
  if (job.removeExternalAbort) {
    job.removeExternalAbort();
    job.removeExternalAbort = null;
  }
}

function canDispatch(job) {
  if (activeJobs.size >= MAX_ACTIVE_READS) return false;
  if (
    job.priority === READ_PRIORITY.BACKGROUND &&
    activeBackgroundCount >= MAX_ACTIVE_BACKGROUND_READS
  ) {
    return false;
  }
  return true;
}

function nextJob() {
  const hasForeground = foregroundQueue.length > 0;
  const hasBackground = backgroundQueue.length > 0;
  if (!hasForeground && !hasBackground) return null;
  if (hasForeground && hasBackground) {
    if (foregroundSinceBackground >= FOREGROUND_BURST) {
      if (canDispatch(backgroundQueue[0])) return backgroundQueue[0];
      return foregroundQueue[0];
    }
    if (canDispatch(foregroundQueue[0])) return foregroundQueue[0];
    return backgroundQueue[0];
  }
  return hasForeground ? foregroundQueue[0] : backgroundQueue[0];
}

function dispatch(job) {
  removeFromQueue(job);
  job.dispatched = true;
  activeJobs.add(job);
  if (job.priority === READ_PRIORITY.BACKGROUND) {
    activeBackgroundCount += 1;
    foregroundSinceBackground = 0;
  } else {
    foregroundSinceBackground += 1;
  }

  Promise.resolve()
    .then(() => job.task(job.controller.signal))
    .then(
      (value) => {
        finishJob(job);
        job.resolve(value);
      },
      (error) => {
        finishJob(job);
        job.reject(error);
      }
    );
}

function finishJob(job) {
  if (job.finished) return;
  job.finished = true;
  activeJobs.delete(job);
  if (job.priority === READ_PRIORITY.BACKGROUND) {
    activeBackgroundCount = Math.max(0, activeBackgroundCount - 1);
  }
  cleanupJob(job);
  drain();
}

function drain() {
  for (;;) {
    if (activeJobs.size >= MAX_ACTIVE_READS) return;
    const job = nextJob();
    if (!job) return;
    if (!canDispatch(job)) return;
    dispatch(job);
  }
}

/**
 * Run an opted-in read under the shared budget.
 *
 * @param {'foreground'|'background'} priority
 * @param {AbortSignal|undefined|null} externalSignal
 * @param {(signal: AbortSignal) => Promise<any>} task
 * @returns {Promise<any>}
 */
export function scheduleRead(priority, externalSignal, task) {
  const normalized =
    priority === READ_PRIORITY.BACKGROUND ? READ_PRIORITY.BACKGROUND : READ_PRIORITY.FOREGROUND;

  const controller = new AbortController();
  /** @type {(value?: any) => void} */
  let resolve = () => {};
  /** @type {(reason?: any) => void} */
  let reject = () => {};
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  /** @type {any} */
  const job = {
    priority: normalized,
    task,
    controller,
    dispatched: false,
    finished: false,
    removeExternalAbort: null,
    resolve,
    reject,
    promise,
  };

  const abortQueuedOrActive = () => {
    if (job.finished) return;
    if (job.dispatched) {
      // Abort the owned physical attempt. Its task settles the job.
      controller.abort();
      return;
    }
    job.finished = true;
    removeFromQueue(job);
    cleanupJob(job);
    reject(abortError());
  };

  if (externalSignal) {
    if (externalSignal.aborted) {
      job.finished = true;
      reject(abortError());
      return promise;
    }
    externalSignal.addEventListener('abort', abortQueuedOrActive, { once: true });
    job.removeExternalAbort = () =>
      externalSignal.removeEventListener('abort', abortQueuedOrActive);
  }

  const queue = queueFor(normalized);
  if (queue.length >= MAX_PENDING_READS) {
    job.finished = true;
    cleanupJob(job);
    reject(queueFullError());
    return promise;
  }

  queue.push(job);
  drain();
  return promise;
}

/**
 * Cancel all queued and active opted-in work. Exposed for explicit teardown
 * and tests; the session layer intentionally does not call it, keeping the
 * scheduler independent of auth.
 */
export function resetReadScheduler() {
  for (const job of [...foregroundQueue, ...backgroundQueue]) {
    if (job.finished) continue;
    job.finished = true;
    cleanupJob(job);
    job.reject(abortError());
  }
  foregroundQueue.length = 0;
  backgroundQueue.length = 0;
  // Abandon active work immediately: abort its owned attempt and release the
  // slot, even if the underlying task ignores the abort signal.
  for (const job of [...activeJobs]) {
    if (job.finished) continue;
    job.finished = true;
    activeJobs.delete(job);
    cleanupJob(job);
    job.controller.abort();
    job.reject(abortError());
  }
  activeBackgroundCount = 0;
  foregroundSinceBackground = 0;
}

/** Snapshot of scheduler occupancy for diagnostics and tests. */
export function getReadSchedulerState() {
  return {
    active: activeJobs.size,
    activeBackground: activeBackgroundCount,
    pending: foregroundQueue.length + backgroundQueue.length,
    pendingForeground: foregroundQueue.length,
    pendingBackground: backgroundQueue.length,
  };
}
