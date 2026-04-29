import { fetchAPI } from './core.js';

// OAuth 2.0 server-side endpoints (frontend consent flow).
//
// /info populates the consent page from the URL's query params (server
// validates the request shape). /approve and /deny mint or refuse the
// authorization code; both return { redirect_to: <url> } that the SPA then
// `window.location.replace`s to bounce the browser back to the third-party
// app.
//
// The /token endpoint is server-to-server and isn't called from the
// frontend — third-party clients hit it directly.
export const oauth = {
  /**
   * Validate the /authorize request and fetch display info for the consent page.
   * @param {URLSearchParams | Record<string,string>} params
   */
  authorizeInfo: (params) => {
    const qs = params instanceof URLSearchParams ? params : new URLSearchParams(params);
    return fetchAPI(`/oauth/authorize/info?${qs.toString()}`);
  },

  /**
   * User clicked Allow. Body mirrors the /authorize query so approval is bound to a specific consent context.
   */
  authorizeApprove: (data) =>
    fetchAPI('/oauth/authorize/approve', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  /**
   * User clicked Deny.
   */
  authorizeDeny: (data) =>
    fetchAPI('/oauth/authorize/deny', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
