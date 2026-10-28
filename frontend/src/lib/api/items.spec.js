import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { items } from './items.js';

function backlogResponse(data = [], pagination = {}) {
  return new Response(JSON.stringify({ data, pagination }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}

function requestURL(fetchMock, call = 0) {
  return new URL(fetchMock.mock.calls[call][0], 'https://windshift.test');
}

describe('backlog item requests', () => {
  let fetchMock;

  beforeEach(() => {
    fetchMock = vi.fn().mockImplementation(() => Promise.resolve(backlogResponse()));
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps completed items excluded unless explicitly requested', async () => {
    await items.getBacklog(7, null, null, { page: 1, limit: 50 });
    expect(requestURL(fetchMock).searchParams.has('include_completed')).toBe(false);

    await items.getBacklog(7, null, null, {
      page: 1,
      limit: 50,
      include_completed: true,
    });
    expect(requestURL(fetchMock, 1).searchParams.get('include_completed')).toBe('true');
  });

  it('uses the same completed scope when resolving the end reorder boundary', async () => {
    fetchMock
      .mockResolvedValueOnce(
        backlogResponse([{ id: 11 }], { page: 1, page_size: 1, total_items: 3 })
      )
      .mockResolvedValueOnce(
        backlogResponse([{ id: 33 }], { page: 3, page_size: 1, total_items: 3 })
      );

    await expect(
      items.getBacklogBoundary(7, null, 'item_type_id != 2', 'end', true)
    ).resolves.toEqual({ id: 33 });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    for (const call of [0, 1]) {
      const params = requestURL(fetchMock, call).searchParams;
      expect(params.get('workspace_id')).toBe('7');
      expect(params.get('sub_ql')).toBe('item_type_id != 2');
      expect(params.get('include_completed')).toBe('true');
      expect(params.get('page_size')).toBe('1');
    }
    expect(requestURL(fetchMock, 0).searchParams.get('page')).toBe('1');
    expect(requestURL(fetchMock, 1).searchParams.get('page')).toBe('3');
  });
});
