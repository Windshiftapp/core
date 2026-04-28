/**
 * Footer link helpers shared by hub and portal stores.
 *
 * Both stores keep a `footerColumns` array of `{ title, links: [{text, url}] }`
 * and persist edits via a `saveCustomizations()` call. The state lives at
 * module scope under Svelte 5 `$state`, so the caller passes a `setColumns`
 * updater that mutates its own variable.
 *
 * Returns: { addFooterLink, removeFooterLink, updateColumnTitle, updateFooterLink }
 */
export function createFooterLinkHelpers({ setColumns, saveCustomizations }) {
  function addFooterLink(columnIndex) {
    setColumns((columns) =>
      columns.map((col, idx) =>
        idx === columnIndex ? { ...col, links: [...col.links, { text: '', url: '' }] } : col
      )
    );
    saveCustomizations();
  }

  function removeFooterLink(columnIndex, linkIndex) {
    setColumns((columns) =>
      columns.map((col, idx) =>
        idx === columnIndex ? { ...col, links: col.links.filter((_, i) => i !== linkIndex) } : col
      )
    );
    saveCustomizations();
  }

  function updateColumnTitle(columnIndex, title) {
    setColumns((columns) =>
      columns.map((col, idx) => (idx === columnIndex ? { ...col, title } : col))
    );
    saveCustomizations();
  }

  function updateFooterLink(columnIndex, linkIndex, field, value) {
    setColumns((columns) =>
      columns.map((col, idx) =>
        idx === columnIndex
          ? {
              ...col,
              links: col.links.map((link, i) =>
                i === linkIndex ? { ...link, [field]: value } : link
              ),
            }
          : col
      )
    );
    saveCustomizations();
  }

  return { addFooterLink, removeFooterLink, updateColumnTitle, updateFooterLink };
}
