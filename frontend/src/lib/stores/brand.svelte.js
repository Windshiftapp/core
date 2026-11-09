import { branding as brandingApi } from '../api/misc.js';
import { APP_NAME } from '../constants.js';
import { surfaceError } from './toasts.svelte.js';

// Instance branding (site title + active theme logos), hydrated at boot from
// the public /branding endpoint so it is available before authentication.
let siteTitle = $state(APP_NAME);
let brandLayout = $state('mark_text');
let logoURL = $state('');
let logoURLDark = $state('');
let logoFullURL = $state('');
let logoFullURLDark = $state('');
let loaded = $state(false);
let loading = $state(false);

function hydrate(value) {
  if (!value) return;
  siteTitle = value.site_title || APP_NAME;
  brandLayout = value.brand_layout || 'mark_text';
  logoURL = value.logo_url || '';
  logoURLDark = value.logo_url_dark || '';
  logoFullURL = value.logo_full_url || '';
  logoFullURLDark = value.logo_full_url_dark || '';
  loaded = true;
  loading = false;
}

async function load(options = {}) {
  if (loaded || loading) return;
  loading = true;
  try {
    hydrate(await brandingApi.get(options));
  } catch (error) {
    surfaceError(error, { silent: true, context: 'loadBranding' });
    // Keep the built-in defaults so the shell still renders.
    loaded = true;
  } finally {
    loading = false;
  }
}

async function reload(options = {}) {
  loaded = false;
  loading = false;
  await load(options);
}

/**
 * Resolve which logo and whether to show the title for the sidebar lockup.
 * The collapsed sidebar always uses the mark; the expanded sidebar follows
 * brand_layout. `src` is null when no custom logo is set, so the caller can
 * fall back to the built-in mark.
 */
export function resolveBrandLockup(
  { isDark = false, expanded = true } = {},
  snapshot = { siteTitle, brandLayout, logoURL, logoURLDark, logoFullURL, logoFullURLDark }
) {
  const mark = (isDark && snapshot.logoURLDark) || snapshot.logoURL || null;
  const full = (isDark && snapshot.logoFullURLDark) || snapshot.logoFullURL || null;
  const text = snapshot.siteTitle || APP_NAME;

  if (!expanded) {
    return { src: mark, variant: 'mark', showText: false, text };
  }
  if (snapshot.brandLayout === 'full_logo') {
    return { src: full || mark, variant: full ? 'full' : 'mark', showText: false, text };
  }
  if (snapshot.brandLayout === 'mark_only') {
    return { src: mark, variant: 'mark', showText: false, text };
  }
  return { src: mark, variant: 'mark', showText: true, text };
}

export const brandStore = {
  get siteTitle() {
    return siteTitle;
  },
  get brandLayout() {
    return brandLayout;
  },
  get logoURL() {
    return logoURL;
  },
  get logoURLDark() {
    return logoURLDark;
  },
  get logoFullURL() {
    return logoFullURL;
  },
  get logoFullURLDark() {
    return logoFullURLDark;
  },
  get loaded() {
    return loaded;
  },
  get loading() {
    return loading;
  },
  get snapshot() {
    return { siteTitle, brandLayout, logoURL, logoURLDark, logoFullURL, logoFullURLDark };
  },
  hydrate,
  load,
  reload,
};
