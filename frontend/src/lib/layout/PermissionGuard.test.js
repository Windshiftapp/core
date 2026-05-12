import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

// The component subscribes to permissionStore (from the stores barrel).
// We replace it with a controllable writable so each test can pick the
// (isSystemAdmin × userPermissionKeys × userPermissions) tuple it wants.
vi.mock('../stores', async () => {
  const { writable } = await import('svelte/store');
  const inner = writable({
    isSystemAdmin: false,
    userPermissionKeys: new Set(),
    userPermissions: new Set(),
  });
  return { permissionStore: inner };
});

// The default fallback path renders an UnauthorizedAccess page; stub it to
// a marker we can find in the DOM. This isolates the guard's logic from
// the page component's styling/copy.
vi.mock('../pages/UnauthorizedAccess.svelte', () => ({
  default: function MockUnauthorized() {
    return {};
  },
}));

// i18n.t — return the key verbatim so assertions don't depend on locale data.
vi.mock('../stores/i18n.svelte.js', () => ({
  t: vi.fn((key) => key),
}));

import { permissionStore } from '../stores';
import PermissionGuard from './PermissionGuard.svelte';

// Build a child snippet that renders a unique sentinel so screen.getByText
// can detect whether the guard let the children through.
function childrenSnippet(text = 'PROTECTED-CONTENT') {
  return createRawSnippet(() => ({
    render: () => `<span data-testid="protected">${text}</span>`,
  }));
}

// Custom fallback that emits the required permission it was given, so we
// can assert the prop threading.
function fallbackSnippet() {
  return createRawSnippet((permParam) => ({
    render: () => {
      const perm = permParam ? permParam() : '';
      return `<span data-testid="fallback">denied:${perm ?? ''}</span>`;
    },
  }));
}

beforeEach(() => {
  permissionStore.set({
    isSystemAdmin: false,
    userPermissionKeys: new Set(),
    userPermissions: new Set(),
  });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('PermissionGuard', () => {
  test('renders children when no permission required', () => {
    render(PermissionGuard, {
      props: {
        children: childrenSnippet(),
      },
    });
    expect(screen.getByTestId('protected')).toBeInTheDocument();
  });

  test('blocks children and shows default fallback when permission key missing', () => {
    render(PermissionGuard, {
      props: {
        permissionKey: 'workspace.edit',
        children: childrenSnippet(),
      },
    });
    expect(screen.queryByTestId('protected')).not.toBeInTheDocument();
  });

  test('renders children when permission key is present', () => {
    permissionStore.set({
      isSystemAdmin: false,
      userPermissionKeys: new Set(['workspace.edit']),
      userPermissions: new Set(),
    });
    render(PermissionGuard, {
      props: {
        permissionKey: 'workspace.edit',
        children: childrenSnippet(),
      },
    });
    expect(screen.getByTestId('protected')).toBeInTheDocument();
  });

  test('system admin bypasses permissionKey check', () => {
    permissionStore.set({
      isSystemAdmin: true,
      userPermissionKeys: new Set(),
      userPermissions: new Set(),
    });
    render(PermissionGuard, {
      props: {
        permissionKey: 'workspace.edit',
        children: childrenSnippet(),
      },
    });
    expect(screen.getByTestId('protected')).toBeInTheDocument();
  });

  test('renders children when permission id is present', () => {
    permissionStore.set({
      isSystemAdmin: false,
      userPermissionKeys: new Set(),
      userPermissions: new Set([42]),
    });
    render(PermissionGuard, {
      props: {
        permissionId: 42,
        children: childrenSnippet(),
      },
    });
    expect(screen.getByTestId('protected')).toBeInTheDocument();
  });

  test('blocks when permission id is not in user permissions set', () => {
    permissionStore.set({
      isSystemAdmin: false,
      userPermissionKeys: new Set(),
      userPermissions: new Set([1, 2, 3]),
    });
    render(PermissionGuard, {
      props: {
        permissionId: 42,
        children: childrenSnippet(),
      },
    });
    expect(screen.queryByTestId('protected')).not.toBeInTheDocument();
  });

  test('requireSystemAdmin only passes for system admins', () => {
    // Without the flag, having any permission key is irrelevant.
    permissionStore.set({
      isSystemAdmin: false,
      userPermissionKeys: new Set(['anything']),
      userPermissions: new Set([1]),
    });
    const { unmount } = render(PermissionGuard, {
      props: {
        requireSystemAdmin: true,
        children: childrenSnippet('SA-ONLY'),
      },
    });
    expect(screen.queryByText('SA-ONLY')).not.toBeInTheDocument();
    unmount();

    permissionStore.set({
      isSystemAdmin: true,
      userPermissionKeys: new Set(),
      userPermissions: new Set(),
    });
    render(PermissionGuard, {
      props: {
        requireSystemAdmin: true,
        children: childrenSnippet('SA-ONLY'),
      },
    });
    expect(screen.getByText('SA-ONLY')).toBeInTheDocument();
  });

  test('custom fallback receives the required permission key', () => {
    render(PermissionGuard, {
      props: {
        permissionKey: 'workspace.delete',
        children: childrenSnippet(),
        fallback: fallbackSnippet(),
      },
    });
    expect(screen.queryByTestId('protected')).not.toBeInTheDocument();
    const fb = screen.getByTestId('fallback');
    expect(fb).toBeInTheDocument();
    expect(fb.textContent).toContain('workspace.delete');
  });

  test('custom fallback gets system.admin label when requireSystemAdmin is the gate', () => {
    render(PermissionGuard, {
      props: {
        requireSystemAdmin: true,
        children: childrenSnippet(),
        fallback: fallbackSnippet(),
      },
    });
    const fb = screen.getByTestId('fallback');
    expect(fb.textContent).toContain('system.admin');
  });
});
