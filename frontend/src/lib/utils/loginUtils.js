import { authenticateWithCredential, getWebAuthnErrorMessage } from './webauthn-utils.js';

export function getBaseLoginState() {
  return {
    emailOrUsername: '',
    password: '',
    rememberMe: false,
    showPassword: false,
    validationError: '',
    fidoAvailable: false,
    tryingFido: false,
    showFidoOption: false
  };
}

export async function evaluateFidoAvailability(api, emailOrUsername) {
  if (!emailOrUsername?.trim()) {
    return { available: false, showOption: false };
  }

  try {
    await api.auth.startFIDOLogin(emailOrUsername.trim());
    return { available: true, showOption: true };
  } catch (error) {
    // Expected errors: 401 (not authenticated), 404 (no FIDO credentials) - don't spam console
    const isExpectedError = error.message?.includes('AUTHENTICATION_REQUIRED') ||
                           error.message?.includes('404') ||
                           error.message?.includes('not found');
    if (!isExpectedError) {
      console.error('FIDO availability check failed:', error);
    }
    return { available: false, showOption: false };
  }
}

export async function performFidoLogin(api, emailOrUsername) {
  if (!emailOrUsername?.trim()) {
    throw new Error('Email or username is required');
  }

  const challengeResponse = await api.auth.startFIDOLogin(emailOrUsername.trim());

  const sessionId = challengeResponse.sessionId;
  const publicKeyOptions = challengeResponse.publicKey || challengeResponse.options || challengeResponse;

  if (!publicKeyOptions || !publicKeyOptions.challenge) {
    throw new Error('Invalid FIDO challenge from server');
  }

  const credentialResponse = await authenticateWithCredential(publicKeyOptions);

  const loginData = sessionId
    ? { sessionId, response: credentialResponse }
    : credentialResponse;

  return api.auth.completeFIDOLogin(loginData);
}

export function deriveFidoError(error) {
  return getWebAuthnErrorMessage(error);
}
