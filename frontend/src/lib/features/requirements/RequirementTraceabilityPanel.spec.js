/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../../api.js';
import RequirementTraceabilityPanel from './RequirementTraceabilityPanel.svelte';

vi.mock('../../api.js', async () => ({
  api: {
    links: {
      getForPage: vi.fn().mockResolvedValue({ incoming: [], outgoing: [] }),
      create: vi.fn().mockResolvedValue({ id: 99 }),
    },
    tests: { testCases: (await vi.importActual('../../api/tests/testCases.js')).testCases },
    linkTypes: {
      getAll: vi.fn().mockResolvedValue([
        { id: 1, builtin_key: 'implements' },
        { id: 2, builtin_key: 'tests' },
        { id: 3, builtin_key: 'specifies' },
        { id: 4, builtin_key: 'relates_to' },
      ]),
    },
    requirements: { listKeys: vi.fn().mockResolvedValue([]) },
  },
}));
vi.mock('../../stores/i18n.svelte.js', () => ({ t: (key) => key }));

describe('RequirementTraceabilityPanel', () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it('shows named link actions and opens the selected search', async () => {
    render(RequirementTraceabilityPanel, { workspaceId: 7, pageId: 42, canEdit: true });

    await fireEvent.click(await screen.findByRole('button', { name: 'items.addLink' }));

    const labels = [
      'requirements.traceability.implementsAdd',
      'requirements.traceability.addTest',
      'requirements.traceability.specifiesAdd',
      'requirements.traceability.addPage',
      'requirements.traceability.addAsset',
    ];
    expect(screen.getAllByRole('menuitem')).toHaveLength(labels.length);
    for (const name of labels) {
      expect(screen.getByRole('menuitem', { name })).toBeVisible();
    }

    await fireEvent.click(screen.getByRole('menuitem', { name: labels[1] }));

    expect(screen.getByRole('textbox', { name: labels[1] })).toHaveAttribute(
      'placeholder',
      'requirements.traceability.searchTests'
    );
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('searches every test folder through the API and links the selected result', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      data: [{ id: 81, title: 'Тест кейс', folder_id: 12 }],
    }), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    render(RequirementTraceabilityPanel, { workspaceId: 7, pageId: 42, canEdit: true });

    await fireEvent.click(await screen.findByRole('button', { name: 'items.addLink' }));
    await fireEvent.click(screen.getByRole('menuitem', { name: 'requirements.traceability.addTest' }));
    await fireEvent.input(screen.getByRole('textbox'), { target: { value: 'Тест ке' } });

    const result = await screen.findByRole('button', { name: 'Тест кейс' });
    expect(result).toBeVisible();
    const request = new URL(fetchMock.mock.calls[0][0], 'https://windshift.test');
    expect(request.pathname).toBe('/api/v2/workspaces/7/test-cases');
    expect(Object.fromEntries(request.searchParams)).toEqual({
      all: 'true', page_size: '10', q: 'Тест ке',
    });

    await fireEvent.click(result);
    await waitFor(() => expect(api.links.create).toHaveBeenCalledWith({
      link_type_id: 2,
      source_type: 'page', source_id: 42,
      target_type: 'test_case', target_id: 81,
    }));
    await waitFor(() => expect(screen.queryByRole('textbox')).not.toBeInTheDocument());
  });
});
