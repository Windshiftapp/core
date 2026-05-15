import { render, screen } from '@testing-library/svelte';
import { beforeAll, describe, expect, test, vi } from 'vitest';

// Mock api + i18n consistently with the main renderer test file. The two
// test files run in isolation so duplicating the mock setup is fine.
vi.mock('../../api.js', () => ({
  api: { getUsers: vi.fn() },
}));
vi.mock('../../stores/i18n.svelte.js', () => ({
  t: (key, params) => (params?.field ? `setField:${params.field}` : key),
  i18n: { locale: 'en-US' },
}));

beforeAll(() => {
  if (!Element.prototype.animate) {
    Element.prototype.animate = () => ({ finished: Promise.resolve(), cancel: () => {} });
  }
});

import CustomFieldRenderer from './CustomFieldRenderer.svelte';

function renderReadonly(props) {
  return render(CustomFieldRenderer, {
    props: { readonly: true, onStartEdit: () => {}, ...props },
  });
}

function renderStatic(props) {
  return render(CustomFieldRenderer, {
    props: { readonly: true, onStartEdit: null, ...props },
  });
}

// Edge cases that complement the main test file. The focus is on
// robustness — situations the renderer probably won't see in normal use
// but must not crash on:
//
//   - Unknown field_type (forward-compat with future field types)
//   - Malformed field.options JSON (corrupt DB row or mid-migration state)
//   - Type-mismatched value (data drift after a field_type change)
//   - Duplicate ids in a multiselect array (no server-side dedup)
//   - URL field with dangerous schemes (XSS guard)

describe('unknown field_type', () => {
  test('falls through to text rendering, no crash', () => {
    renderReadonly({
      field: { field_type: 'totally-new-type', name: 'Future field' },
      value: 'some value',
    });
    expect(screen.getByText('some value')).toBeInTheDocument();
  });

  test('unknown type with object value falls through to "[object Object]"', () => {
    // Documents current behavior: an object value reaches the generic
    // <span>{renderDisplayValue()}</span> branch which calls String(v) →
    // "[object Object]". The renderer should arguably check for the
    // unknown-type case more carefully; pinning this so a future fix
    // (or a future regression) is visible.
    const { container } = renderReadonly({
      field: { field_type: 'totally-new-type', name: 'Future field' },
      value: { a: 1 },
    });
    expect(container.textContent).toContain('[object Object]');
  });
});

describe('malformed field.options', () => {
  test('select with broken JSON options falls back to raw id string', () => {
    // parseFieldOptions in optionUtils.js catches the JSON.parse error
    // and returns an empty option list. resolveOptionLabel then misses
    // and returns String(value). The end user sees a bare id, not a
    // crash.
    renderReadonly({
      field: { field_type: 'select', name: 'Priority', options: '{not valid json' },
      value: 2,
    });
    expect(screen.getByText('2')).toBeInTheDocument();
  });

  test('multiselect with broken JSON options renders ids as comma-separated strings', () => {
    renderReadonly({
      field: { field_type: 'multiselect', name: 'Tags', options: 'definitely not json' },
      value: [1, 2],
    });
    expect(screen.getByText('1, 2')).toBeInTheDocument();
  });

  test('multiselect with duplicate ids in the value array renders duplicates (no dedup)', () => {
    // Server-side doesn't dedupe (see backend custom_field_edge_cases_test.go);
    // the renderer mirrors that — each id is resolved independently and
    // joined with ", ".
    const opts = JSON.stringify({
      next_id: 3,
      items: [
        { id: 1, label: 'Bug' },
        { id: 2, label: 'Feature' },
      ],
    });
    renderReadonly({
      field: { field_type: 'multiselect', name: 'Tags', options: opts },
      value: [1, 1, 2],
    });
    expect(screen.getByText('Bug, Bug, Feature')).toBeInTheDocument();
  });
});

describe('type-mismatched values (data drift)', () => {
  test('checkbox with the string "false" renders as common.yes (truthy coercion)', () => {
    // Subtle quirk: the renderer uses `v ? yes : no`. The string "false"
    // is truthy in JS, so a checkbox field with stale string data shows
    // as "yes". Pinning to flag this if the renderer adds smarter
    // boolean coercion later.
    renderReadonly({
      field: { field_type: 'checkbox', name: 'Done' },
      value: 'false',
    });
    expect(screen.getByText('common.yes')).toBeInTheDocument();
  });

  test('checkbox with the number 0 renders as common.no (falsy)', () => {
    renderStatic({
      field: { field_type: 'checkbox', name: 'Done' },
      value: 0,
    });
    // 0 hits the OUTER value-not-empty guard? Let's see: 0 !== null,
    // 0 !== undefined, 0 !== ''. So it enters the per-type branch and
    // renders v ? yes : no → "no". This documents that 0 is treated
    // as a meaningful "no" (the renderer does NOT short-circuit on 0).
    expect(screen.getByText('common.no')).toBeInTheDocument();
  });

  test('number field given an integer (not a string) still renders', () => {
    renderReadonly({
      field: { field_type: 'number', name: 'Estimate' },
      value: 42,
    });
    expect(screen.getByText('42')).toBeInTheDocument();
  });

  test('select with a string-encoded id resolves like the numeric form', () => {
    const opts = JSON.stringify({
      next_id: 3,
      items: [
        { id: 1, label: 'Low' },
        { id: 2, label: 'Medium' },
      ],
    });
    renderReadonly({
      field: { field_type: 'select', name: 'Priority', options: opts },
      value: '2',
    });
    expect(screen.getByText('Medium')).toBeInTheDocument();
  });
});

describe('URL field with dangerous schemes (XSS guard)', () => {
  test('javascript: URL is displayed as text but not used as the href', () => {
    renderStatic({
      field: { field_type: 'url', name: 'Link' },
      value: 'javascript:alert(1)',
    });
    const link = screen.getByRole('link');
    expect(link).toHaveTextContent('javascript:alert(1)');
    expect(link.getAttribute('href')).toBe('#');
  });

  test('data: URL is displayed as text but not used as the href', () => {
    renderStatic({
      field: { field_type: 'url', name: 'Link' },
      value: 'data:text/html,<script>alert(1)</script>',
    });
    const link = screen.getByRole('link');
    expect(link).toHaveTextContent('data:text/html,<script>alert(1)</script>');
    expect(link.getAttribute('href')).toBe('#');
  });

  test('protocol-relative URL is displayed as text but not used as the href', () => {
    renderStatic({
      field: { field_type: 'url', name: 'Link' },
      value: '//evil.example/path',
    });
    const link = screen.getByRole('link');
    expect(link).toHaveTextContent('//evil.example/path');
    expect(link.getAttribute('href')).toBe('#');
  });

  test('safe https URL renders unchanged', () => {
    renderStatic({
      field: { field_type: 'url', name: 'Link' },
      value: 'https://example.com/safe',
    });
    expect(screen.getByRole('link').getAttribute('href')).toBe('https://example.com/safe');
  });
});

describe('rename of an option label — renderer reflects the new label', () => {
  test('an item that stored option id=1 picks up the renamed label automatically', () => {
    // This is the visible side of the backend "renaming propagates"
    // test in custom_field_edge_cases_test.go. The renderer looks up
    // labels by id from field.options at render time, so a new options
    // payload with the same id but a different label renders the new
    // label without any item-side change. Pin this so a future shift
    // toward "store labels on items" trips the test.
    const original = JSON.stringify({
      next_id: 4,
      items: [
        { id: 1, label: 'Low' },
        { id: 2, label: 'Med' },
        { id: 3, label: 'High' },
      ],
    });
    const renamed = JSON.stringify({
      next_id: 4,
      items: [
        { id: 1, label: 'Trivial' },
        { id: 2, label: 'Med' },
        { id: 3, label: 'High' },
      ],
    });

    const { unmount } = renderReadonly({
      field: { field_type: 'select', name: 'Priority', options: original },
      value: 1,
    });
    expect(screen.getByText('Low')).toBeInTheDocument();
    unmount();

    renderReadonly({
      field: { field_type: 'select', name: 'Priority', options: renamed },
      value: 1,
    });
    expect(screen.getByText('Trivial')).toBeInTheDocument();
    expect(screen.queryByText('Low')).not.toBeInTheDocument();
  });
});

describe('duplicate option labels with different ids', () => {
  test('renderer shows the label of whichever id was selected (no de-dup)', () => {
    // Two options with the same label but different ids — a config the
    // admin shouldn't really create but the schema doesn't prevent.
    const opts = JSON.stringify({
      next_id: 3,
      items: [
        { id: 1, label: 'Open' },
        { id: 2, label: 'Open' },
      ],
    });
    renderReadonly({
      field: { field_type: 'select', name: 'Status', options: opts },
      value: 2,
    });
    // Both ids resolve to "Open" so the user can't tell them apart in
    // the display. This isn't a bug per se — it's a config issue — but
    // it's surprising behavior worth pinning.
    expect(screen.getByText('Open')).toBeInTheDocument();
  });
});
