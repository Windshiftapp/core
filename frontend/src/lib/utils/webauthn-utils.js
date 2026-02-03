/**
 * WebAuthn utility functions for FIDO2 authentication
 * Centralizes browser WebAuthn operations and encoding/decoding
 */

/**
 * Convert base64url string to ArrayBuffer
 * @param {string} base64url - Base64URL encoded string
 * @returns {ArrayBuffer} - Decoded ArrayBuffer
 */
export function base64urlToArrayBuffer(base64url) {
  // Replace URL-safe characters with standard base64 characters
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');

  // Add padding if necessary
  const padding = (4 - (base64.length % 4)) % 4;
  const paddedBase64 = base64 + '='.repeat(padding);

  // Decode base64 to binary string
  const binaryString = atob(paddedBase64);

  // Convert binary string to ArrayBuffer
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }

  return bytes.buffer;
}

/**
 * Convert ArrayBuffer to base64url string
 * @param {ArrayBuffer} buffer - ArrayBuffer to encode
 * @returns {string} - Base64URL encoded string
 */
export function arrayBufferToBase64url(buffer) {
  // Convert ArrayBuffer to binary string
  const bytes = new Uint8Array(buffer);
  let binaryString = '';
  for (let i = 0; i < bytes.byteLength; i++) {
    binaryString += String.fromCharCode(bytes[i]);
  }

  // Encode binary string as base64
  const base64 = btoa(binaryString);

  // Convert to base64url by replacing characters and removing padding
  return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

/**
 * Check if WebAuthn is supported by the browser
 * @returns {boolean} - True if WebAuthn is supported
 */
export function isWebAuthnSupported() {
  return !!(
    window.PublicKeyCredential?.isUserVerifyingPlatformAuthenticatorAvailable &&
    window.PublicKeyCredential.isConditionalMediationAvailable
  );
}

/**
 * Check if platform authenticator (passkey) is available
 * @returns {Promise<boolean>} - True if platform authenticator is available
 */
export async function isPlatformAuthenticatorAvailable() {
  if (!isWebAuthnSupported()) {
    return false;
  }

  try {
    return await window.PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable();
  } catch (err) {
    console.error('Error checking platform authenticator:', err);
    return false;
  }
}

/**
 * Check if conditional mediation (autofill) is available
 * @returns {Promise<boolean>} - True if conditional mediation is available
 */
export async function isConditionalMediationAvailable() {
  if (!isWebAuthnSupported()) {
    return false;
  }

  try {
    return await window.PublicKeyCredential.isConditionalMediationAvailable();
  } catch (err) {
    console.error('Error checking conditional mediation:', err);
    return false;
  }
}

/**
 * Prepare credential creation options from server response
 * @param {object} options - Options from server
 * @returns {object} - Prepared options for navigator.credentials.create()
 */
export function prepareCredentialCreationOptions(options) {
  // Handle both old format (direct) and new format (publicKey wrapper)
  const publicKey = options.publicKey || options;

  // Convert challenge from base64url to ArrayBuffer
  publicKey.challenge = base64urlToArrayBuffer(publicKey.challenge);

  // Convert user ID from base64url to ArrayBuffer
  if (publicKey.user?.id) {
    publicKey.user.id = base64urlToArrayBuffer(publicKey.user.id);
  }

  // Convert excluded credentials
  if (publicKey.excludeCredentials) {
    publicKey.excludeCredentials = publicKey.excludeCredentials.map((cred) => ({
      ...cred,
      id: base64urlToArrayBuffer(cred.id),
    }));
  }

  return { publicKey };
}

/**
 * Prepare credential request options from server response
 * @param {object} options - Options from server
 * @returns {object} - Prepared options for navigator.credentials.get()
 */
export function prepareCredentialRequestOptions(options) {
  // Handle both old format (direct) and new format (publicKey wrapper)
  const publicKey = options.publicKey || options;

  // Convert challenge from base64url to ArrayBuffer
  publicKey.challenge = base64urlToArrayBuffer(publicKey.challenge);

  // Convert allowed credentials
  if (publicKey.allowCredentials) {
    publicKey.allowCredentials = publicKey.allowCredentials.map((cred) => ({
      ...cred,
      id: base64urlToArrayBuffer(cred.id),
    }));
  }

  return { publicKey };
}

/**
 * Process credential creation response for sending to server
 * @param {PublicKeyCredential} credential - Credential from navigator.credentials.create()
 * @returns {object} - Processed response for server
 */
export function processCredentialCreationResponse(credential) {
  return {
    id: credential.id,
    rawId: arrayBufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      attestationObject: arrayBufferToBase64url(credential.response.attestationObject),
      clientDataJSON: arrayBufferToBase64url(credential.response.clientDataJSON),
      transports: credential.response.getTransports ? credential.response.getTransports() : [],
    },
  };
}

/**
 * Process credential request response for sending to server
 * @param {PublicKeyCredential} credential - Credential from navigator.credentials.get()
 * @returns {object} - Processed response for server
 */
export function processCredentialRequestResponse(credential) {
  const response = {
    id: credential.id,
    rawId: arrayBufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      authenticatorData: arrayBufferToBase64url(credential.response.authenticatorData),
      clientDataJSON: arrayBufferToBase64url(credential.response.clientDataJSON),
      signature: arrayBufferToBase64url(credential.response.signature),
    },
  };

  // Include userHandle if present (for discoverable credentials)
  if (credential.response.userHandle) {
    response.response.userHandle = arrayBufferToBase64url(credential.response.userHandle);
  }

  return response;
}

/**
 * Register a new WebAuthn credential
 * @param {object} creationOptions - Options from server
 * @returns {Promise<object>} - Processed credential response
 */
export async function registerCredential(creationOptions) {
  if (!isWebAuthnSupported()) {
    throw new Error('WebAuthn is not supported by this browser');
  }

  try {
    // Prepare options
    const options = prepareCredentialCreationOptions(creationOptions);

    // Create credential
    const credential = await navigator.credentials.create(options);

    if (!credential) {
      throw new Error('Failed to create credential');
    }

    // Process response
    return processCredentialCreationResponse(credential);
  } catch (error) {
    if (error.name === 'NotAllowedError') {
      throw new Error('Registration was cancelled or timed out');
    }
    if (error.name === 'InvalidStateError') {
      throw new Error('An authenticator is already registered');
    }
    throw error;
  }
}

/**
 * Authenticate with WebAuthn credential
 * @param {object} requestOptions - Options from server
 * @param {boolean} conditional - Whether to use conditional UI (autofill)
 * @returns {Promise<object>} - Processed credential response
 */
export async function authenticateWithCredential(requestOptions, conditional = false) {
  if (!isWebAuthnSupported()) {
    throw new Error('WebAuthn is not supported by this browser');
  }

  try {
    // Prepare options
    const options = prepareCredentialRequestOptions(requestOptions);

    // Add conditional mediation if requested
    if (conditional) {
      options.mediation = 'conditional';
    }

    // Get credential
    const credential = await navigator.credentials.get(options);

    if (!credential) {
      throw new Error('Failed to get credential');
    }

    // Process response
    return processCredentialRequestResponse(credential);
  } catch (error) {
    if (error.name === 'NotAllowedError') {
      throw new Error('Authentication was cancelled or timed out');
    }
    throw error;
  }
}

/**
 * Get a user-friendly error message for WebAuthn errors
 * @param {Error} error - The error object
 * @returns {string} - User-friendly error message
 */
export function getWebAuthnErrorMessage(error) {
  if (error.name === 'NotAllowedError') {
    return 'The operation was cancelled or timed out. Please try again.';
  }
  if (error.name === 'InvalidStateError') {
    return 'This authenticator is already registered.';
  }
  if (error.name === 'NotSupportedError') {
    return 'Your browser does not support this type of authenticator.';
  }
  if (error.name === 'SecurityError') {
    return 'The operation is not allowed due to security restrictions.';
  }
  if (error.message) {
    return error.message;
  }
  return 'An unexpected error occurred. Please try again.';
}

/**
 * Abort a WebAuthn operation
 * @param {AbortController} controller - The abort controller for the operation
 */
export function abortWebAuthnOperation(controller) {
  if (controller) {
    controller.abort();
  }
}
