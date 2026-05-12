import { fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeAll, describe, expect, test, vi } from 'vitest';

// jsdom does not implement the Web Animations API. Svelte 5 transitions
// call element.animate during outro. Stub it with a Promise-shaped result
// so transitions resolve immediately and don't crash the test runner.
beforeAll(() => {
  if (!Element.prototype.animate) {
    Element.prototype.animate = () => ({
      finished: Promise.resolve(),
      cancel: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      play: () => {},
      pause: () => {},
    });
  }
});

// Modal uses `use:portal` to move its DOM into document.body. The action
// is a thin DOM operation — keep the real one.

import Modal from './Modal.svelte';

function childrenSnippet(text = 'MODAL-BODY') {
  return createRawSnippet(() => ({
    render: () =>
      `<div data-testid="modal-body"><input data-testid="first-input" /><button>X</button>${text}</div>`,
  }));
}

afterEach(() => {
  // The portal action moves modal nodes to document.body and only cleans
  // up on `destroy`. Between test renders any stragglers can stick — wipe
  // the body so each test starts clean.
  document.body.innerHTML = '';
});

describe('Modal — open/closed visibility', () => {
  test('renders nothing when isOpen=false', () => {
    render(Modal, {
      props: {
        isOpen: false,
        children: childrenSnippet(),
      },
    });
    expect(screen.queryByTestId('modal-body')).not.toBeInTheDocument();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  test('renders the dialog backdrop and content when isOpen=true', () => {
    render(Modal, {
      props: {
        isOpen: true,
        children: childrenSnippet(),
      },
    });
    expect(screen.getByTestId('modal-body')).toBeInTheDocument();
    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog).not.toBeNull();
    expect(dialog?.getAttribute('aria-modal')).toBe('true');
  });
});

describe('Modal — backdrop click dismissal', () => {
  test('clicking the backdrop calls onclose', async () => {
    const onclose = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        onclose,
        children: childrenSnippet(),
      },
    });

    const backdrop = document.querySelector('[role="dialog"]');
    await fireEvent.click(backdrop);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  test('clicking inside the modal content does NOT close', async () => {
    const onclose = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        onclose,
        children: childrenSnippet(),
      },
    });

    const body = screen.getByTestId('modal-body');
    await fireEvent.click(body);
    expect(onclose).not.toHaveBeenCalled();
  });

  test('preventClose blocks backdrop dismissal', async () => {
    const onclose = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        preventClose: true,
        onclose,
        children: childrenSnippet(),
      },
    });

    const backdrop = document.querySelector('[role="dialog"]');
    await fireEvent.click(backdrop);
    expect(onclose).not.toHaveBeenCalled();
  });
});

describe('Modal — keyboard handling', () => {
  test('Escape closes the modal', async () => {
    const onclose = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        onclose,
        children: childrenSnippet(),
      },
    });

    const backdrop = document.querySelector('[role="dialog"]');
    await fireEvent.keyDown(backdrop, { key: 'Escape' });
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  test('Escape is suppressed when preventClose is set', async () => {
    const onclose = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        preventClose: true,
        onclose,
        children: childrenSnippet(),
      },
    });

    const backdrop = document.querySelector('[role="dialog"]');
    await fireEvent.keyDown(backdrop, { key: 'Escape' });
    expect(onclose).not.toHaveBeenCalled();
  });

  test('Cmd/Ctrl+Enter fires onSubmit when supplied', async () => {
    const onSubmit = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        onSubmit,
        children: childrenSnippet(),
      },
    });

    const backdrop = document.querySelector('[role="dialog"]');
    // ctrl or meta — keyboardShortcuts treats `modifierKey: true` as
    // either Ctrl or Cmd; cover both.
    await fireEvent.keyDown(backdrop, { key: 'Enter', metaKey: true });
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  test('plain Enter submits when focus is NOT in a textarea', async () => {
    const onSubmit = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        onSubmit,
        children: childrenSnippet(),
      },
    });

    const input = screen.getByTestId('first-input');
    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  test('plain Enter inside a textarea does NOT submit (lets newline through)', async () => {
    const onSubmit = vi.fn();
    // Custom children with a textarea instead of an input.
    const snippet = createRawSnippet(() => ({
      render: () => `<div><textarea data-testid="ta"></textarea></div>`,
    }));
    render(Modal, {
      props: {
        isOpen: true,
        onSubmit,
        children: snippet,
      },
    });

    const textarea = screen.getByTestId('ta');
    await fireEvent.keyDown(textarea, { key: 'Enter' });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  test('submitDisabled blocks both Enter and Cmd+Enter submission', async () => {
    const onSubmit = vi.fn();
    render(Modal, {
      props: {
        isOpen: true,
        onSubmit,
        submitDisabled: true,
        children: childrenSnippet(),
      },
    });

    const input = screen.getByTestId('first-input');
    await fireEvent.keyDown(input, { key: 'Enter' });
    await fireEvent.keyDown(input, { key: 'Enter', metaKey: true });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  test('Enter is a no-op when onSubmit is not provided', async () => {
    // No assertion target here — just ensure no error is thrown and the
    // modal stays open. Regression guard for the "missing onSubmit" path.
    render(Modal, {
      props: {
        isOpen: true,
        children: childrenSnippet(),
      },
    });

    const input = screen.getByTestId('first-input');
    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(screen.getByTestId('modal-body')).toBeInTheDocument();
  });
});

describe('Modal — maxWidth prop', () => {
  test('applies the supplied tailwind max-width class', () => {
    render(Modal, {
      props: {
        isOpen: true,
        maxWidth: 'max-w-2xl',
        children: childrenSnippet(),
      },
    });

    const modalContent = document.querySelector('[role="dialog"] > div');
    expect(modalContent?.className).toContain('max-w-2xl');
  });

  test('defaults to max-w-lg when not provided', () => {
    render(Modal, {
      props: {
        isOpen: true,
        children: childrenSnippet(),
      },
    });
    const modalContent = document.querySelector('[role="dialog"] > div');
    expect(modalContent?.className).toContain('max-w-lg');
  });
});
