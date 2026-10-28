import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ getBacklog: vi.fn() }));

vi.mock('../../api/items.js', () => ({
  items: {
    getBacklog: mocks.getBacklog,
  },
}));
vi.mock('../../api/collections.js', () => ({ collections: {} }));

import { fetchCollectionBacklog } from './collectionService.js';

describe('fetchCollectionBacklog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getBacklog.mockResolvedValue({
      data: [{ id: 1 }],
      pagination: { page: 1, page_size: 25, total_items: 1 },
      meta: { watermark: 9 },
    });
  });

  it('preserves filters and forwards the completed visibility scope', async () => {
    await expect(
      fetchCollectionBacklog(7, null, {
        page: 1,
        limit: 25,
        sub_ql: 'item_type_id != 2',
        include_completed: true,
      })
    ).resolves.toMatchObject({ items: [{ id: 1 }], watermark: 9 });

    expect(mocks.getBacklog).toHaveBeenCalledWith(7, null, null, {
      page: 1,
      limit: 25,
      sub_ql: 'item_type_id != 2',
      omit_descriptions: true,
      include_watermark: true,
      include_completed: true,
      requestOptions: {},
    });
  });
});
