/**
 * Server clock offset detection and correction.
 *
 * Compares the Date header from HTTP responses against the client clock
 * to compute a rolling offset, so all frontend "now" references stay
 * consistent with server-stamped timestamps.
 */

const SAMPLE_COUNT = 5;
let samples = [];
let clockOffset = 0; // ms, positive = server ahead of client

/**
 * Record a new server-vs-client offset sample from an HTTP Date header.
 * Maintains a rolling median of the last SAMPLE_COUNT samples.
 * @param {string} serverDateHeader - RFC 1123 Date header value
 */
export function updateOffset(serverDateHeader) {
  if (!serverDateHeader) return;

  const serverTime = new Date(serverDateHeader).getTime();
  if (Number.isNaN(serverTime)) return;

  const clientTime = Date.now();
  samples.push(serverTime - clientTime);

  if (samples.length > SAMPLE_COUNT) {
    samples = samples.slice(-SAMPLE_COUNT);
  }

  // Median filters out one-off network jitter better than mean
  const sorted = [...samples].sort((a, b) => a - b);
  clockOffset = sorted[Math.floor(sorted.length / 2)];
}

/**
 * Return a Date representing "now" on the server's clock.
 * Before any samples are collected this falls back to the client clock.
 * @returns {Date}
 */
export function serverNow() {
  return new Date(Date.now() + clockOffset);
}

/**
 * Raw offset in milliseconds (positive = server ahead).
 * @returns {number}
 */
export function getClockOffset() {
  return clockOffset;
}

/**
 * True when the absolute offset exceeds 30 seconds.
 * @returns {boolean}
 */
export function isClockDriftSignificant() {
  return Math.abs(clockOffset) > 30_000;
}

/**
 * Number of offset samples collected so far.
 * Useful for deciding when the estimate has stabilised.
 * @returns {number}
 */
export function getSampleCount() {
  return samples.length;
}
