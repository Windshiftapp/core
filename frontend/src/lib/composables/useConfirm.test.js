import { get } from 'svelte/store';
import { afterEach, describe, expect, test } from 'vitest';
import { confirm, confirmDialog } from './useConfirm.js';

afterEach(() => {
  // Reset the global dialog state between tests so order doesn't matter.
  confirmDialog.set({
    show: false,
    title: '',
    message: '',
    confirmText: 'Confirm',
    cancelText: 'Cancel',
    variant: 'danger',
    icon: null,
    onConfirm: null,
    onCancel: null,
  });
});

describe('confirm() — object form', () => {
  test('sets show=true with the supplied fields and applies defaults', () => {
    const promise = confirm({
      title: 'Delete item',
      message: 'This cannot be undone.',
      confirmText: 'Yes, delete',
      variant: 'danger',
    });

    const state = get(confirmDialog);
    expect(state.show).toBe(true);
    expect(state.title).toBe('Delete item');
    expect(state.message).toBe('This cannot be undone.');
    expect(state.confirmText).toBe('Yes, delete');
    expect(state.cancelText).toBe('Cancel'); // default
    expect(state.variant).toBe('danger');
    expect(typeof state.onConfirm).toBe('function');
    expect(typeof state.onCancel).toBe('function');

    // Resolve so the test doesn't leave a hanging promise.
    state.onCancel();
    return expect(promise).resolves.toBe(false);
  });

  test('fills sensible defaults for missing fields', async () => {
    const promise = confirm({});
    const state = get(confirmDialog);
    expect(state.title).toBe('Confirm Action');
    expect(state.message).toBe('Are you sure you want to proceed?');
    expect(state.confirmText).toBe('Confirm');
    expect(state.cancelText).toBe('Cancel');
    expect(state.variant).toBe('danger');
    state.onCancel();
    await promise;
  });

  test('threads icon prop when supplied', async () => {
    const ICON = { name: 'trash' };
    const promise = confirm({ icon: ICON });
    expect(get(confirmDialog).icon).toBe(ICON);
    get(confirmDialog).onCancel();
    await promise;
  });
});

describe('confirm() — string form', () => {
  test('treats (title, message) string args as the dialog content', async () => {
    const promise = confirm('Heads up', 'Really do this?');
    const state = get(confirmDialog);
    expect(state.title).toBe('Heads up');
    expect(state.message).toBe('Really do this?');
    state.onConfirm();
    await expect(promise).resolves.toBe(true);
  });

  test('falls back to default message when only title is provided', async () => {
    const promise = confirm('Just a title');
    expect(get(confirmDialog).title).toBe('Just a title');
    expect(get(confirmDialog).message).toBe('Are you sure you want to proceed?');
    get(confirmDialog).onCancel();
    await promise;
  });
});

describe('confirm() promise resolution', () => {
  test('onConfirm resolves the promise with true and hides the dialog', async () => {
    const promise = confirm('x');
    expect(get(confirmDialog).show).toBe(true);

    get(confirmDialog).onConfirm();

    await expect(promise).resolves.toBe(true);
    expect(get(confirmDialog).show).toBe(false);
  });

  test('onCancel resolves the promise with false and hides the dialog', async () => {
    const promise = confirm('x');
    expect(get(confirmDialog).show).toBe(true);

    get(confirmDialog).onCancel();

    await expect(promise).resolves.toBe(false);
    expect(get(confirmDialog).show).toBe(false);
  });

  test('subsequent confirm() replaces the previous dialog state', async () => {
    // Existing test convention: each confirm() supersedes the prior one
    // (the writable store has a single slot). The first promise stays
    // pending until something resolves it — for the matrix of "dialog
    // currently showing", the latest wins.
    const first = confirm('first');
    const firstHandle = get(confirmDialog).onConfirm;
    const second = confirm('second');
    expect(get(confirmDialog).title).toBe('second');

    // Cancel both to keep the test deterministic.
    get(confirmDialog).onCancel();
    await second;
    // The first promise's callbacks were overwritten — call the captured
    // handle directly to resolve it and avoid hanging.
    firstHandle();
    await expect(first).resolves.toBe(true);
  });
});

describe('confirmDialog store shape', () => {
  test('initial state has show=false', () => {
    const state = get(confirmDialog);
    expect(state.show).toBe(false);
  });

  test('confirm() flips show=true; resolving flips back to false', async () => {
    const promise = confirm('x');
    expect(get(confirmDialog).show).toBe(true);
    get(confirmDialog).onCancel();
    await promise;
    expect(get(confirmDialog).show).toBe(false);
  });
});
