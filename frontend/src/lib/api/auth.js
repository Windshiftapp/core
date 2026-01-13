import { fetchAPI } from './core.js';

export const auth = {
  login: (credentials) => fetchAPI('/auth/login', {
    method: 'POST',
    body: JSON.stringify(credentials),
  }),
  startFIDOLogin: (emailOrUsername) => fetchAPI('/auth/webauthn/login/start', {
    method: 'POST',
    body: JSON.stringify({ email_or_username: emailOrUsername }),
  }),
  completeFIDOLogin: (credentialData) => fetchAPI('/auth/webauthn/login/complete', {
    method: 'POST',
    body: JSON.stringify(credentialData),
  }),
  logout: () => fetchAPI('/auth/logout', {
    method: 'POST',
  }),
  logoutAll: () => fetchAPI('/auth/logout-all', {
    method: 'POST',
  }),
  getCurrentUser: () => fetchAPI('/auth/me'),
  refreshSession: (data = {}) => fetchAPI('/auth/refresh', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  changePassword: (data) => fetchAPI('/auth/change-password', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  // Email verification endpoints
  verifyEmail: (token) => fetchAPI(`/auth/verify-email?token=${encodeURIComponent(token)}`),
  resendVerification: () => fetchAPI('/auth/resend-verification', {
    method: 'POST',
  }),
  getVerificationStatus: () => fetchAPI('/auth/verification-status'),
};
