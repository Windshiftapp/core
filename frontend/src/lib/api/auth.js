import { fetchAPI, fetchAPIV2 } from './core.js';

export const auth = {
  login: (credentials, options = {}) =>
    fetchAPI('/auth/login', {
      ...options,
      method: 'POST',
      body: JSON.stringify(credentials),
    }),
  startFIDOLogin: (emailOrUsername, options = {}) =>
    fetchAPIV2('/auth/passkey/start', {
      ...options,
      method: 'POST',
      body: JSON.stringify({ email_or_username: emailOrUsername }),
    }),
  completeFIDOLogin: (credentialData, options = {}) =>
    fetchAPIV2('/auth/passkey/complete', {
      ...options,
      method: 'POST',
      body: JSON.stringify(credentialData),
    }),
  logout: () =>
    fetchAPI('/auth/logout', {
      method: 'POST',
    }),
  logoutAll: () =>
    fetchAPI('/auth/logout-all', {
      method: 'POST',
    }),
  getCurrentUser: (options) => fetchAPI('/auth/me', options),
  refreshSession: (data = {}) =>
    fetchAPI('/auth/refresh', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  changePassword: (data) =>
    fetchAPI('/auth/change-password', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Email verification endpoints
  verifyEmail: (token) => fetchAPI(`/auth/verify-email?token=${encodeURIComponent(token)}`),
  resendVerification: () =>
    fetchAPI('/auth/resend-verification', {
      method: 'POST',
    }),
  getVerificationStatus: () => fetchAPI('/auth/verification-status'),
  // Invitation endpoints
  verifyInvitation: (token) =>
    fetchAPI(`/auth/invitations/verify?token=${encodeURIComponent(token)}`),
  acceptInvitation: (data) =>
    fetchAPI('/auth/invitations/accept', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
