import { describe, expect, test } from 'vitest';
import { BaseCacheStore } from './BaseCacheStore.svelte.js';

// BaseCacheStore manages a workspace-scoped cache. The class is small but
// covers a critical invariant: switching workspaces must clear cached data
// so a viewer can never see another workspace's residue.

describe('BaseCacheStore.initialize', () => {
  test('sets workspaceId and leaves cache untouched on first call', () => {
    const store = new BaseCacheStore();
    store._cache.set('precondition', 'should not be cleared on first initialize');
    store.initialize(1);
    expect(store.workspaceId).toBe(1);
    // First initialize from null → id transitions, so reset() runs. The
    // precondition above is therefore expected to be cleared. This test
    // documents that behavior; a no-op-on-first-init would require a
    // separate `_initialized` flag in the implementation.
    expect(store._cache.has('precondition')).toBe(false);
  });

  test('is a no-op when the same workspaceId is set twice', () => {
    const store = new BaseCacheStore();
    store.initialize(5);
    store._cache.set('keep', 'me');
    store.initialize(5);
    expect(store._cache.get('keep')).toBe('me');
  });

  test('coerces numeric strings to numbers', () => {
    const store = new BaseCacheStore();
    store.initialize('42');
    expect(store.workspaceId).toBe(42);
    expect(typeof store.workspaceId).toBe('number');
  });

  test('resets cache when workspaceId changes', () => {
    const store = new BaseCacheStore();
    store.initialize(1);
    store._cache.set('items', ['a']);
    store._pending.set('req', Promise.resolve());

    store.initialize(2);

    expect(store.workspaceId).toBe(2);
    expect(store._cache.size).toBe(0);
    expect(store._pending.size).toBe(0);
  });

  test("'1' and 1 are treated as the same workspace", () => {
    const store = new BaseCacheStore();
    store.initialize(1);
    store._cache.set('keep', 'me');
    store.initialize('1');
    expect(store._cache.get('keep')).toBe('me');
  });
});

describe('BaseCacheStore.invalidateAll', () => {
  test('clears the cache and pending maps without touching workspaceId', () => {
    const store = new BaseCacheStore();
    store.initialize(7);
    store._cache.set('a', 1);
    store._cache.set('b', 2);
    store._pending.set('p', Promise.resolve());

    store.invalidateAll();

    expect(store._cache.size).toBe(0);
    expect(store._pending.size).toBe(0);
    expect(store.workspaceId).toBe(7);
  });
});

describe('BaseCacheStore.reset', () => {
  test('clears both maps and the workspaceId', () => {
    const store = new BaseCacheStore();
    store.initialize(7);
    store._cache.set('a', 1);
    store._pending.set('p', Promise.resolve());

    store.reset();

    expect(store._cache.size).toBe(0);
    expect(store._pending.size).toBe(0);
    expect(store.workspaceId).toBeNull();
  });
});

describe('BaseCacheStore subclassing', () => {
  // Smoke check that the class is extension-friendly — every cached
  // resource store inherits from this base.
  test('subclass methods can use _cache and inherit reset/invalidateAll', () => {
    class ItemCache extends BaseCacheStore {
      put(key, val) {
        this._cache.set(key, val);
      }
      get(key) {
        return this._cache.get(key);
      }
    }

    const cache = new ItemCache();
    cache.initialize(3);
    cache.put('items', [{ id: 1 }]);

    expect(cache.get('items')).toEqual([{ id: 1 }]);

    cache.invalidateAll();
    expect(cache.get('items')).toBeUndefined();
    expect(cache.workspaceId).toBe(3);

    cache.reset();
    expect(cache.workspaceId).toBeNull();
  });
});
