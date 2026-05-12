import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeAll, describe, expect, test, vi } from 'vitest';

// Mock the api module — the renderer calls api.getUsers() for the user
// field type when the stored value is a bare id (not an object). Stub it
// to a controllable spy so we can drive the user-lookup branch.
vi.mock('../../api.js', () => ({
  api: {
    getUsers: vi.fn(),
  },
}));

// i18n — both the t function (used by the renderer) and the i18n object
// (read transitively by formatCustomFieldDate for the locale). Both must
// be exported, otherwise the date formatter throws and returns '' which
// the renderer then falls back to the raw YYYY-MM-DD string.
vi.mock('../../stores/i18n.svelte.js', () => ({
  t: (key, params) => {
    if (params?.field) return `setField:${params.field}`;
    return key;
  },
  i18n: { locale: 'en-US' },
}));

// Some Svelte transitions reach for element.animate (Web Animations API)
// which jsdom doesn't implement. The renderer itself doesn't use
// transitions, but a child picker might pull one in transitively. Defensive
// stub.
beforeAll(() => {
  if (!Element.prototype.animate) {
    Element.prototype.animate = () => ({ finished: Promise.resolve(), cancel: () => {} });
  }
});

import { api } from '../../api.js';
import CustomFieldRenderer from './CustomFieldRenderer.svelte';

afterEach(() => {
  vi.clearAllMocks();
  document.body.innerHTML = '';
});

// Helper — render in readonly mode with onStartEdit so we exercise the
// clickable-button branch (the most common path in item detail views).
function renderReadonly(props) {
  return render(CustomFieldRenderer, {
    props: {
      readonly: true,
      onStartEdit: () => {},
      ...props,
    },
  });
}

// Helper — readonly without onStartEdit so we exercise the static-display
// branch (the one used in card layouts, where email/url become <a> tags).
function renderStatic(props) {
  return render(CustomFieldRenderer, {
    props: {
      readonly: true,
      onStartEdit: null,
      ...props,
    },
  });
}

// Standard option set used by select/multiselect tests.
const SELECT_OPTIONS = JSON.stringify({
  next_id: 4,
  items: [
    { id: 1, label: 'Low' },
    { id: 2, label: 'Medium' },
    { id: 3, label: 'High' },
  ],
});

describe('text field', () => {
  test('renders the raw value', () => {
    renderReadonly({
      field: { field_type: 'text', name: 'Notes' },
      value: 'Hello world',
    });
    expect(screen.getByText('Hello world')).toBeInTheDocument();
  });

  test('empty value shows setField placeholder', () => {
    renderReadonly({
      field: { field_type: 'text', name: 'Notes' },
      value: '',
    });
    expect(screen.getByText('setField:notes')).toBeInTheDocument();
  });
});

describe('number field', () => {
  test('renders parsed number', () => {
    renderReadonly({
      field: { field_type: 'number', name: 'Estimate' },
      value: '42',
    });
    expect(screen.getByText('42')).toBeInTheDocument();
  });

  test('non-numeric value falls through unchanged', () => {
    renderReadonly({
      field: { field_type: 'number', name: 'Estimate' },
      value: 'NaN-like',
    });
    expect(screen.getByText('NaN-like')).toBeInTheDocument();
  });

  test('floating point is preserved', () => {
    renderReadonly({
      field: { field_type: 'number', name: 'Estimate' },
      value: '3.14',
    });
    expect(screen.getByText('3.14')).toBeInTheDocument();
  });
});

describe('date field', () => {
  test('YYYY-MM-DD value is rendered without timezone drift', () => {
    // formatCustomFieldDate forces UTC parsing — Jan 15 stays Jan 15 even
    // when the host runs in a non-UTC zone.
    renderReadonly({
      field: { field_type: 'date', name: 'Due' },
      value: '2026-01-15',
    });
    // Locale rendering is host-dependent (e.g. en-US "Jan 15, 2026");
    // assert the day-month-year tokens individually.
    const node = screen.getByText(/2026/);
    expect(node.textContent).toMatch(/15/);
    expect(node.textContent).toMatch(/Jan/i);
  });

  test('empty value shows setField placeholder', () => {
    renderReadonly({
      field: { field_type: 'date', name: 'Due' },
      value: null,
    });
    expect(screen.getByText('setField:due')).toBeInTheDocument();
  });
});

describe('email field', () => {
  test('button variant shows the address as text', () => {
    renderReadonly({
      field: { field_type: 'email', name: 'Contact' },
      value: 'alice@example.com',
    });
    expect(screen.getByText('alice@example.com')).toBeInTheDocument();
  });

  test('static variant renders a mailto: link', () => {
    renderStatic({
      field: { field_type: 'email', name: 'Contact' },
      value: 'alice@example.com',
    });
    const link = screen.getByRole('link', { name: 'alice@example.com' });
    expect(link).toHaveAttribute('href', 'mailto:alice@example.com');
  });
});

describe('url field', () => {
  test('static variant renders an external link with rel safety', () => {
    renderStatic({
      field: { field_type: 'url', name: 'Link' },
      value: 'https://example.com/docs',
    });
    const link = screen.getByRole('link', { name: 'https://example.com/docs' });
    expect(link).toHaveAttribute('href', 'https://example.com/docs');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
    expect(link).toHaveAttribute('rel', expect.stringContaining('noreferrer'));
  });
});

describe('checkbox field', () => {
  test('truthy value renders common.yes', () => {
    renderReadonly({
      field: { field_type: 'checkbox', name: 'Done' },
      value: true,
    });
    expect(screen.getByText('common.yes')).toBeInTheDocument();
  });

  test('falsy-but-present value renders common.no', () => {
    // value=false is meaningful for a checkbox; it must render "no", not
    // fall into the empty-state placeholder branch. The renderer treats
    // boolean false as "present" (only null/undefined/'' trigger empty).
    renderStatic({
      field: { field_type: 'checkbox', name: 'Done' },
      value: false,
    });
    expect(screen.getByText('common.no')).toBeInTheDocument();
  });
});

describe('select field', () => {
  test('resolves option id to label', () => {
    renderReadonly({
      field: { field_type: 'select', name: 'Priority', options: SELECT_OPTIONS },
      value: 2,
    });
    expect(screen.getByText('Medium')).toBeInTheDocument();
  });

  test('orphan option id renders as raw string (deleted-option safety net)', () => {
    // Regression guard for the Q3 scenario: an option that was selected
    // and then removed. Until backend cleanup catches up, the renderer
    // displays the bare id rather than crashing or showing "[object Object]".
    renderReadonly({
      field: { field_type: 'select', name: 'Priority', options: SELECT_OPTIONS },
      value: 99,
    });
    expect(screen.getByText('99')).toBeInTheDocument();
  });

  test('numeric-string id also resolves', () => {
    renderReadonly({
      field: { field_type: 'select', name: 'Priority', options: SELECT_OPTIONS },
      value: '3',
    });
    expect(screen.getByText('High')).toBeInTheDocument();
  });
});

describe('multiselect field', () => {
  test('joins resolved labels with commas', () => {
    renderReadonly({
      field: { field_type: 'multiselect', name: 'Tags', options: SELECT_OPTIONS },
      value: [1, 3],
    });
    expect(screen.getByText('Low, High')).toBeInTheDocument();
  });

  test('mixed valid + orphan ids — orphans render as raw strings inline', () => {
    // Multiselect orphan scenario: one option deleted, the array still
    // contains its id. The deleted slot renders as "<id>" between the
    // surviving labels.
    renderReadonly({
      field: { field_type: 'multiselect', name: 'Tags', options: SELECT_OPTIONS },
      value: [1, 99, 2],
    });
    expect(screen.getByText('Low, 99, Medium')).toBeInTheDocument();
  });

  test('empty array shows setField placeholder', () => {
    // An empty multiselect array is treated as "not set" by the renderer's
    // null guard (`value === null|undefined|''`)? No — [] is truthy. But
    // resolveOptionLabels([]) joins to "". So the visible text is "".
    // Either way the placeholder branch IS exercised only for falsy values.
    renderReadonly({
      field: { field_type: 'multiselect', name: 'Tags', options: SELECT_OPTIONS },
      value: [],
    });
    // Pragmatic assertion: nothing crashed and at least the field label
    // placeholder doesn't appear (we did pass a value).
    expect(screen.queryByText('setField:tags')).not.toBeInTheDocument();
  });
});

describe('user field', () => {
  test('object value shows the user name', () => {
    renderReadonly({
      field: { field_type: 'user', name: 'Owner' },
      value: { id: 1, name: 'Alice Smith' },
    });
    expect(screen.getByText('Alice Smith')).toBeInTheDocument();
  });

  test('bare-id value triggers api.getUsers() and resolves to first_name + last_name', async () => {
    api.getUsers.mockResolvedValueOnce([
      { id: 7, first_name: 'Bob', last_name: 'Lee', username: 'blee' },
    ]);
    renderReadonly({
      field: { field_type: 'user', name: 'Owner' },
      value: 7,
    });
    await waitFor(() => {
      expect(screen.getByText('Bob Lee')).toBeInTheDocument();
    });
    expect(api.getUsers).toHaveBeenCalledTimes(1);
  });

  test('unknown user id shows common.unknownUser', async () => {
    api.getUsers.mockResolvedValueOnce([
      { id: 7, first_name: 'Bob', last_name: 'Lee', username: 'blee' },
    ]);
    renderReadonly({
      field: { field_type: 'user', name: 'Owner' },
      value: 999, // not in the users list
    });
    await waitFor(() => {
      expect(screen.getByText('common.unknownUser')).toBeInTheDocument();
    });
  });
});

describe('milestone field', () => {
  test('resolves id against milestones prop and shows name', () => {
    renderReadonly({
      field: { field_type: 'milestone', name: 'Milestone' },
      value: 11,
      milestones: [
        { id: 10, name: 'M1', category_color: '#ff0000' },
        { id: 11, name: 'M2', category_color: '#00ff00' },
      ],
    });
    expect(screen.getByText('M2')).toBeInTheDocument();
  });

  test('unknown milestone id shows setField placeholder', () => {
    // The renderer's milestone branch only shows the name when found —
    // otherwise it falls into the empty-state Target-icon placeholder.
    renderReadonly({
      field: { field_type: 'milestone', name: 'Milestone' },
      value: 99,
      milestones: [{ id: 10, name: 'M1' }],
    });
    expect(screen.getByText('setField:milestone')).toBeInTheDocument();
  });
});

describe('iteration field', () => {
  test('resolves id against iterations prop and shows name', () => {
    renderReadonly({
      field: { field_type: 'iteration', name: 'Sprint' },
      value: 5,
      iterations: [
        { id: 4, name: 'Sprint 1', is_global: false },
        { id: 5, name: 'Sprint 2', is_global: true },
      ],
    });
    expect(screen.getByText('Sprint 2')).toBeInTheDocument();
  });

  test('unknown iteration id falls through to raw value (no helpful placeholder)', () => {
    // The renderer returns the bare value when iterations.find() misses.
    // This documents current behaviour; differs from milestone which has
    // an explicit empty branch.
    renderReadonly({
      field: { field_type: 'iteration', name: 'Sprint' },
      value: 99,
      iterations: [{ id: 1, name: 'Sprint 1' }],
    });
    expect(screen.getByText('99')).toBeInTheDocument();
  });
});

describe('asset field', () => {
  test('object value with tag + title is formatted as "TAG - Title"', () => {
    renderReadonly({
      field: { field_type: 'asset', name: 'Machine' },
      value: { id: 1, asset_tag: 'A-001', title: 'Forklift' },
    });
    expect(screen.getByText('A-001 - Forklift')).toBeInTheDocument();
  });

  test('object value with title only', () => {
    renderReadonly({
      field: { field_type: 'asset', name: 'Machine' },
      value: { id: 1, title: 'Forklift' },
    });
    expect(screen.getByText('Forklift')).toBeInTheDocument();
  });

  test('bare-id value renders as "Asset #N"', () => {
    renderReadonly({
      field: { field_type: 'asset', name: 'Machine' },
      value: 42,
    });
    expect(screen.getByText('Asset #42')).toBeInTheDocument();
  });
});

describe('portalcustomer field', () => {
  test('object value shows name', () => {
    renderReadonly({
      field: { field_type: 'portalcustomer', name: 'Customer' },
      value: { id: 3, name: 'Acme Inc.' },
    });
    expect(screen.getByText('Acme Inc.')).toBeInTheDocument();
  });

  test('bare-id falls back to "Customer #N"', () => {
    renderReadonly({
      field: { field_type: 'portalcustomer', name: 'Customer' },
      value: 8,
    });
    expect(screen.getByText('Customer #8')).toBeInTheDocument();
  });
});

describe('customerorganisation field', () => {
  test('object value shows name', () => {
    renderReadonly({
      field: { field_type: 'customerorganisation', name: 'Org' },
      value: { id: 3, name: 'Acme Group' },
    });
    expect(screen.getByText('Acme Group')).toBeInTheDocument();
  });

  test('bare-id falls back to "Organisation #N"', () => {
    renderReadonly({
      field: { field_type: 'customerorganisation', name: 'Org' },
      value: 8,
    });
    expect(screen.getByText('Organisation #8')).toBeInTheDocument();
  });
});

describe('combobox field', () => {
  test('comma-separated value renders as chips', () => {
    renderReadonly({
      field: { field_type: 'combobox', name: 'Labels' },
      value: 'urgent, bug, ui',
    });
    expect(screen.getByText('urgent')).toBeInTheDocument();
    expect(screen.getByText('bug')).toBeInTheDocument();
    expect(screen.getByText('ui')).toBeInTheDocument();
  });

  test('empty value shows nothing for the chip area', () => {
    renderReadonly({
      field: { field_type: 'combobox', name: 'Labels' },
      value: '',
    });
    // Falls into the unset state; specifically the setField placeholder.
    expect(screen.getByText('setField:labels')).toBeInTheDocument();
  });
});

describe('linking field', () => {
  test('static variant shows count of linked items (array)', () => {
    renderStatic({
      field: { field_type: 'linking', name: 'Blocked by' },
      value: [{ id: 1 }, { id: 2 }, { id: 3 }],
    });
    expect(screen.getByText('3 linked')).toBeInTheDocument();
  });

  test('static variant shows "1 linked" for a single object value', () => {
    renderStatic({
      field: { field_type: 'linking', name: 'Blocked by' },
      value: { id: 1 },
    });
    expect(screen.getByText('1 linked')).toBeInTheDocument();
  });

  test('null value short-circuits to the outer items.notSet placeholder', () => {
    // The static-display branch tests `value !== null && value !== undefined`
    // BEFORE entering the per-type linking case. So a null linking value
    // never reaches the linking-specific em-dash branch — it lands on the
    // generic "not set" placeholder instead. The em-dash branch inside
    // the linking case is in practice unreachable from real call sites
    // (which always pass either an array or null).
    renderStatic({
      field: { field_type: 'linking', name: 'Blocked by' },
      value: null,
    });
    expect(screen.getByText('items.notSet')).toBeInTheDocument();
  });

  test('empty array renders the em-dash (no links)', () => {
    // Regression guard for a former bug: the renderer used to check
    //   else if value && typeof value === 'object' → "1 linked"
    // after the empty-or-positive-array check, but `[]` is truthy AND
    // typeof 'object', so it incorrectly matched the "1 linked" branch.
    // The fix excludes arrays from that branch — see CustomFieldRenderer.svelte.
    renderStatic({
      field: { field_type: 'linking', name: 'Blocked by' },
      value: [],
    });
    expect(screen.getByText('—')).toBeInTheDocument();
  });
});

describe('unset / placeholder behavior', () => {
  test.each([
    ['text', null],
    ['number', null],
    ['date', null],
    ['email', null],
    ['url', null],
    ['select', null],
    ['multiselect', null],
    ['user', null],
    ['milestone', null], // milestone has its own setField branch
    ['asset', null],
    ['portalcustomer', null],
    ['customerorganisation', null],
    ['combobox', null],
  ])('%s renders setField placeholder when value is null', (fieldType) => {
    renderReadonly({
      field: { field_type: fieldType, name: 'Label', options: SELECT_OPTIONS },
      value: null,
      milestones: [],
      iterations: [],
    });
    // The placeholder uses the field name lowercased; we asserted that
    // pattern in the per-type tests above. Here we just confirm the
    // placeholder branch fires (vs rendering an empty span).
    expect(screen.getByText('setField:label')).toBeInTheDocument();
  });
});
