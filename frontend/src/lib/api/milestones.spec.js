import { afterEach, describe, expect, it, vi } from 'vitest';
import { milestones } from './milestones.js';

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('milestones comments client', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reads one page of the comment feed, passing page size and cursor', async () => {
    const feed = {
      comments: [{ id: 2, content: 'second' }],
      next_cursor: 'older-than-2',
      has_more: true,
      total: 3,
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ data: feed }))
      .mockResolvedValueOnce(jsonResponse({ data: { comments: [], has_more: false } }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(milestones.getComments(7, { limit: 25 })).resolves.toEqual(feed);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/milestones/7/comments?page_size=25');

    await milestones.getComments(7, { limit: 25, cursor: 'older-than-2' });
    expect(fetchMock.mock.calls[1][0]).toBe(
      '/api/v2/milestones/7/comments?page_size=25&cursor=older-than-2'
    );
  });

  it('creates a comment with only its Markdown content', async () => {
    const created = { id: 3, milestone_id: 7, content: '**hi**' };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ data: created }, 201));
    vi.stubGlobal('fetch', fetchMock);

    await expect(milestones.createComment(7, '**hi**')).resolves.toEqual(created);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v2/milestones/7/comments');
    expect(options.method).toBe('POST');
    expect(JSON.parse(options.body)).toEqual({ content: '**hi**' });
  });

  it('updates a comment with a merge patch and deletes it by milestone and id', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ data: { id: 3, content: 'edited' } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await milestones.updateComment(7, 3, 'edited');
    const [patchURL, patchOptions] = fetchMock.mock.calls[0];
    expect(patchURL).toBe('/api/v2/milestones/7/comments/3');
    expect(patchOptions.method).toBe('PATCH');
    expect(new Headers(patchOptions.headers).get('Content-Type')).toBe(
      'application/merge-patch+json'
    );
    expect(JSON.parse(patchOptions.body)).toEqual({ content: 'edited' });

    await milestones.deleteComment(7, 3);
    const [deleteURL, deleteOptions] = fetchMock.mock.calls[1];
    expect(deleteURL).toBe('/api/v2/milestones/7/comments/3');
    expect(deleteOptions.method).toBe('DELETE');
  });
});

describe('milestones page links client', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('lists, links, and unlinks pages on a milestone', async () => {
    const link = { id: 9, milestone_id: 7, page_id: 42, page_title: 'Spec' };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ data: [link] }))
      .mockResolvedValueOnce(jsonResponse({ data: link }, 201))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(milestones.getPageLinks(7)).resolves.toEqual([link]);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/milestones/7/page-links');

    await expect(milestones.linkPage(7, 42)).resolves.toEqual(link);
    const [createURL, createOptions] = fetchMock.mock.calls[1];
    expect(createURL).toBe('/api/v2/milestones/7/page-links');
    expect(createOptions.method).toBe('POST');
    expect(JSON.parse(createOptions.body)).toEqual({ page_id: 42 });

    await milestones.unlinkPage(7, 9);
    const [deleteURL, deleteOptions] = fetchMock.mock.calls[2];
    expect(deleteURL).toBe('/api/v2/milestones/7/page-links/9');
    expect(deleteOptions.method).toBe('DELETE');
  });
});

describe('milestones activity client', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reads one page of the activity feed and whether more follow', async () => {
    const entry = { id: 'item_comment:3', type: 'item_comment_added' };
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ data: { entries: [entry], has_more: true } }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(milestones.getActivity(7, { page: 2, pageSize: 20 })).resolves.toEqual({
      entries: [entry],
      hasMore: true,
    });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/milestones/7/activity?page=2&page_size=20');
  });

  it('reports no more entries when has_more is false or missing', async () => {
    const entry = { id: 'milestone_comment:5', type: 'milestone_comment_added' };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ data: { entries: [entry], has_more: false } }))
      .mockResolvedValueOnce(jsonResponse({ data: { entries: [] } }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(milestones.getActivity(7)).resolves.toEqual({
      entries: [entry],
      hasMore: false,
    });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/milestones/7/activity?page=1&page_size=50');

    await expect(milestones.getActivity(7, { page: 3 })).resolves.toEqual({
      entries: [],
      hasMore: false,
    });
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v2/milestones/7/activity?page=3&page_size=50');
  });
});
