import { auth } from '../api/auth.js';
import { authenticateWithCredential, getWebAuthnErrorMessage } from './webauthn-utils.js';
export async function performFidoLogin(emailOrUsername, rememberMe = false, options = {}) {
  const { signal, conditional = false, onCredential } = options;
  signal?.throwIfAborted();
  const challengeResponse = await auth.startFIDOLogin(emailOrUsername?.trim() || '', { signal });
  signal?.throwIfAborted();

  const sessionId = challengeResponse.sessionId;
  const publicKeyOptions =
    challengeResponse.publicKey || challengeResponse.options || challengeResponse;

  if (!publicKeyOptions?.challenge) {
    throw new Error('Invalid FIDO challenge from server');
  }

  const credentialResponse = await authenticateWithCredential(
    publicKeyOptions,
    conditional,
    signal
  );
  signal?.throwIfAborted();
  onCredential?.();

  const loginData = sessionId
    ? {
        sessionId,
        response: credentialResponse,
        remember_me: options.getRememberMe?.() ?? rememberMe,
      }
    : credentialResponse;

  return auth.completeFIDOLogin(loginData, { signal });
}

export function deriveFidoError(error) {
  return getWebAuthnErrorMessage(error);
}
