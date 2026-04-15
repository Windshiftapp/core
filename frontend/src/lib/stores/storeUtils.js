/**
 * Utility helpers for Svelte writable stores.
 */

/**
 * Synchronously read the current value of a Svelte store.
 * @template T
 * @param {import('svelte/store').Readable<T>} store
 * @returns {T}
 */
export function getStoreValue(store) {
  let value;
  store.subscribe((v) => (value = v))();
  return value;
}

/**
 * Set every store in the list to null.
 * @param  {...import('svelte/store').Writable<any>} stores
 */
export function clearStores(...stores) {
  for (const store of stores) {
    store.set(null);
  }
}

/**
 * Mixin that adds drag-state management methods to a class-based store.
 * The target class must have `fieldDragState` (Map) and `draggedField` properties.
 * Call `applyDragStateMixin(MyClass)` after the class definition.
 *
 * @param {Function} TargetClass - The class to extend with drag-state methods
 */
export function applyDragStateMixin(TargetClass) {
  TargetClass.prototype.setDragState = function (fieldId, state) {
    this.fieldDragState.set(fieldId, state);
    this.fieldDragState = new Map(this.fieldDragState);
  };

  TargetClass.prototype.clearDragState = function () {
    this.fieldDragState.forEach((_, id) => {
      this.fieldDragState.set(id, { closestEdge: null });
    });
    this.fieldDragState = new Map(this.fieldDragState);
  };

  TargetClass.prototype.setDraggedField = function (field) {
    this.draggedField = field;
  };

  TargetClass.prototype.clearDraggedField = function () {
    this.draggedField = null;
  };
}
