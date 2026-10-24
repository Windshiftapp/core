import { links, linkTypes } from '../../api/configuration.js';

/**
 * Linked work items for the page open in the right pane — the other side
 * of the item↔page links touching it. Loaded alongside the page itself;
 * mutated optimistically by the popover button's add/remove callbacks.
 */
export function createPageLinksState() {
  let pageLinks = $state(/** @type {any[]} */ ([]));
  let loading = $state(false);
  // The page id this state is scoped to. Responses for anything else are
  // dropped so a fast page switch can't write the wrong links back.
  let activePageId = null;
  // Plain counter — not reactive, only used to token requests.
  let requestSeq = 0;

  // System link types — used to locate the "Page" type id so the popover
  // can pass it to POST /links. One fetch per session is plenty; the
  // result is small and stable.
  let linkTypesCache = $state(/** @type {any[]} */ ([]));
  let pageLinkTypeId = $derived(linkTypesCache.find((lt) => lt?.name === 'Page')?.id ?? null);

  async function loadFor(pageId) {
    activePageId = pageId;
    const seq = ++requestSeq;
    loading = true;
    try {
      const resp = await links.getForPage(pageId);
      if (seq !== requestSeq || activePageId !== pageId) return;
      const outgoing = Array.isArray(resp?.outgoing) ? resp.outgoing : [];
      const incoming = Array.isArray(resp?.incoming) ? resp.incoming : [];
      // De-dup by id; outgoing and incoming should never share rows but
      // belt-and-braces against backend changes.
      const seen = new Set();
      const merged = [];
      for (const link of [...incoming, ...outgoing]) {
        if (link && link.id != null && !seen.has(link.id)) {
          seen.add(link.id);
          merged.push(link);
        }
      }
      pageLinks = merged;
    } catch (err) {
      if (seq !== requestSeq || activePageId !== pageId) return;
      console.error('failed to load page links', err);
      pageLinks = [];
    } finally {
      if (seq === requestSeq) loading = false;
    }
  }

  async function ensureLinkTypesLoaded() {
    if (linkTypesCache.length > 0) return;
    try {
      const resp = await linkTypes.getAll();
      linkTypesCache = Array.isArray(resp) ? resp : (resp?.data ?? []);
    } catch (err) {
      console.error('failed to load link types', err);
    }
  }

  function handleCreated(link) {
    if (link && !pageLinks.some((l) => l.id === link.id)) {
      pageLinks = [link, ...pageLinks];
    }
    if (activePageId != null) {
      // Re-fetch to pick up joined fields the server applies on read
      // (status_name, item_type_icon, workspace_key) that the POST
      // response doesn't include.
      loadFor(activePageId);
    }
  }

  function handleRemoved(linkId) {
    pageLinks = pageLinks.filter((l) => l.id !== linkId);
  }

  /** Drop state and in-flight responses when the selection clears. */
  function clear() {
    requestSeq++;
    activePageId = null;
    pageLinks = [];
    loading = false;
  }

  return {
    get pageLinks() {
      return pageLinks;
    },
    get loading() {
      return loading;
    },
    get pageLinkTypeId() {
      return pageLinkTypeId;
    },
    loadFor,
    ensureLinkTypesLoaded,
    handleCreated,
    handleRemoved,
    clear,
  };
}
