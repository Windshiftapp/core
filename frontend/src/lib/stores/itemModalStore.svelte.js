/**
 * Single global work-item detail modal.
 *
 * Views call `open(itemId, workspaceId)` instead of each mounting their own
 * `<ItemDetail isModal>`. `MainAppOverlays` renders the one host, so two
 * instances can never fight over the shared `itemDetailStore`.
 */
class ItemModalStore {
  itemId = $state(null);
  workspaceId = $state(null);

  get isOpen() {
    return this.itemId != null;
  }

  open(itemId, workspaceId = null) {
    if (itemId == null) return;
    this.itemId = itemId;
    this.workspaceId = workspaceId;
  }

  close() {
    this.itemId = null;
    this.workspaceId = null;
  }
}

export const itemModalStore = new ItemModalStore();
