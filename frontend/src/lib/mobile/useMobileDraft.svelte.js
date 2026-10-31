import { setNavigationInterceptor } from '../router.js';

/**
 * Shared unsaved-draft handling for the mobile create and edit pages.
 *
 * Both guard navigation with a confirm sheet while dirty and persist the
 * title/description to sessionStorage so a reload or app kill does not lose the
 * draft. The pages differ only in how they decide dirtiness and which values
 * they hydrate, so those are injected.
 *
 * @param {{
 *   draftKey: () => string,
 *   isDirty: () => boolean,
 *   isSaving: () => boolean,
 *   values: () => { title: string, description: string },
 *   applyDraft: (draft: { title?: string, description?: string }) => void,
 *   shouldWrite: () => boolean,
 *   canHydrate?: () => boolean,
 * }} config
 */
export function useMobileDraft({
  draftKey,
  isDirty,
  isSaving,
  values,
  applyDraft,
  shouldWrite,
  canHydrate = () => true,
}) {
  let confirmDiscardOpen = $state(false);
  let hydratedKey = $state(null);
  let draftRetired = false;

  // Leaving with unsaved input asks for confirmation, via the header Cancel and
  // via the back gesture (router interceptor). A discarded draft releases the
  // interceptor so the pending navigation can proceed.
  $effect(() => {
    setNavigationInterceptor(() => {
      if (draftRetired || !isDirty()) return false;
      confirmDiscardOpen = true;
      return true;
    });
    return () => setNavigationInterceptor(null);
  });

  // Restore a draft once per key, after the page is ready to receive it.
  $effect(() => {
    const key = draftKey();
    if (!canHydrate() || hydratedKey === key) return;
    hydratedKey = key;
    try {
      const raw = sessionStorage.getItem(key);
      if (!raw) return;
      applyDraft(JSON.parse(raw));
    } catch {
      /* corrupt draft — start clean */
    }
  });

  // Write the draft back as the user types; remove it once the values clear.
  $effect(() => {
    const key = draftKey();
    if (!hydratedKey || isSaving() || draftRetired) return;
    const { title, description } = values();
    try {
      if (shouldWrite()) {
        sessionStorage.setItem(key, JSON.stringify({ title, description }));
      } else {
        sessionStorage.removeItem(key);
      }
    } catch {
      /* storage unavailable — drafts are best-effort */
    }
  });

  function clearDraft() {
    // Retire the write-back effect first: when `saving` flips to false in the
    // save's finally, the effect must not resurrect the draft before unmount.
    draftRetired = true;
    try {
      sessionStorage.removeItem(draftKey());
    } catch {
      /* ignore */
    }
  }

  return {
    get confirmDiscardOpen() {
      return confirmDiscardOpen;
    },
    set confirmDiscardOpen(value) {
      confirmDiscardOpen = value;
    },
    clearDraft,
  };
}
