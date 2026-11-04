/**
 * Server-provided runtime state injected into the SPA shell by the Go server.
 * See internal/server/index_html.go for the producer.
 */

const BASE_URL_KEY = '__WINDSHIFT_BASE_URL__';
const BASE_URL_CONFIGURED_KEY = '__WINDSHIFT_BASE_URL_CONFIGURED__';

/**
 * The public base URL the server uses for absolute links and OAuth callbacks.
 * This is the value callbacks are actually built from, so admin screens should
 * prefer it over the browser origin. Returns null when the server did not
 * inject it (unit tests, non-Go hosts).
 *
 * @returns {string | null}
 */
export function serverBaseURL() {
  if (typeof window === 'undefined') return null;
  const raw = /** @type {any} */ (window)[BASE_URL_KEY];
  if (typeof raw !== 'string') return null;
  return raw.replace(/\/+$/, '');
}

/**
 * Whether BASE_URL/--base-url was explicitly configured on the server.
 * Returns null when the server did not inject the value.
 *
 * @returns {boolean | null}
 */
export function isServerBaseURLConfigured() {
  if (typeof window === 'undefined') return null;
  const raw = /** @type {any} */ (window)[BASE_URL_CONFIGURED_KEY];
  return typeof raw === 'boolean' ? raw : null;
}
