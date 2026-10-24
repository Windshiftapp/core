import { contextPath, toExternal } from '../runtime/contextPath.js';
import { isTauri } from '../utils/isTauri.js';
import { DESKTOP_MODAL_FEATURES, DESKTOP_OPEN_MODAL_EVENT } from './events.js';

const supportedModals = new Set(DESKTOP_MODAL_FEATURES);

// The custom-scheme callback the desktop shell is registered to receive. Must
// match the backend's nativeRedirectURI and the desktop deep-link scheme (WI-446).
export const NATIVE_SSO_REDIRECT_URI = 'windshift://oauth/callback';

// desktopStartSso hands an SSO login URL to the system browser via the desktop
// shell, instead of navigating the embedded webview. The IdP login page —
// especially passkey/WebAuthn — can't run inside the Tauri webview (WKWebView
// doesn't expose WebAuthn), so the real browser does the login and returns to
// the app through the windshift:// deep link, which the shell exchanges for a
// session. loginPath is the relative SSO start path (e.g. /api/sso/login/slug).
export async function desktopStartSso(loginPath) {
  // Absolute URL the system browser can open, carrying the native callback.
  const abs = new URL(toExternal(loginPath), window.location.origin);
  abs.searchParams.set('redirect_uri', NATIVE_SSO_REDIRECT_URI);
  const { invoke } = await import('@tauri-apps/api/core');
  await invoke('desktop_start_sso', { url: abs.href });
}

class DesktopBridge {
  modal = $state(null);
  initialized = false;
  unlisten = null;

  async init() {
    if (this.initialized || !isTauri()) return;
    this.initialized = true;

    try {
      const [{ listen }, { invoke }] = await Promise.all([
        import('@tauri-apps/api/event'),
        import('@tauri-apps/api/core'),
      ]);

      this.unlisten = await listen(DESKTOP_OPEN_MODAL_EVENT, (event) => {
        const modal = event?.payload?.modal;
        this.open(modal);
      });

      await invoke('set_webapp_ui_ready', {
        features: DESKTOP_MODAL_FEATURES,
        basePath: contextPath(),
      });
    } catch (err) {
      console.warn('[desktop-bridge] init failed:', err);
    }
  }

  open(modal) {
    if (!supportedModals.has(modal)) {
      console.warn('[desktop-bridge] ignoring unsupported modal:', modal);
      return;
    }
    this.modal = modal;
  }

  close() {
    this.modal = null;
  }
}

export const desktopBridge = new DesktopBridge();
