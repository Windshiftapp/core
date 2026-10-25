/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import MilestoneComments from './MilestoneComments.svelte';

const mocks = vi.hoisted(() => ({
  getComments: vi.fn(),
  canEditOthersComments: vi.fn(),
}));

vi.mock('../../api/milestones.js', () => ({
  milestones: { getComments: mocks.getComments },
}));
vi.mock('../../stores/auth.svelte.js', () => ({
  authStore: { currentUser: { id: 1, first_name: 'Ada' } },
}));
vi.mock('../../stores/i18n.svelte.js', () => ({ t: (key) => key }));
vi.mock('../../stores/workspacePermissions.svelte.js', () => ({
  workspacePermissions: { canEditOthersComments: mocks.canEditOthersComments },
}));
vi.mock('../../composables/useConfirm.js', () => ({ confirm: vi.fn() }));
// The Markdown editor is irrelevant to ordering; render nothing for it.
vi.mock('../../editors/LazyMilkdownEditor.svelte', () => ({
  default: () => {},
}));

describe('MilestoneComments', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // The API returns the newest page first.
    mocks.getComments.mockResolvedValue({
      comments: [
        {
          id: 3,
          author_id: 1,
          content: 'third',
          created_at: '2026-01-03T10:00:00Z',
        },
        {
          id: 2,
          author_id: 2,
          content: 'second',
          created_at: '2026-01-02T10:00:00Z',
        },
        {
          id: 1,
          author_id: 1,
          content: 'first',
          created_at: '2026-01-01T10:00:00Z',
        },
      ],
      has_more: false,
      total: 3,
    });
  });

  afterEach(() => cleanup());

  it('lists comments newest first below the composer', async () => {
    render(MilestoneComments, { milestoneId: 50 });

    const rows = await screen.findAllByTestId('milestone-comment');
    expect(rows.map((row) => row.dataset.commentId)).toEqual(['3', '2', '1']);

    const composer = screen.getByTestId('milestone-comment-editor');
    expect(
      composer.compareDocumentPosition(rows[0]) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
    expect(mocks.getComments).toHaveBeenCalledWith(50, { limit: 25 });
    expect(screen.queryByTestId('milestone-comments-load-more')).not.toBeInTheDocument();
  });

  it('loads older comments with the next cursor and appends them', async () => {
    mocks.getComments
      .mockResolvedValueOnce({
        comments: [
          {
            id: 3,
            author_id: 1,
            content: 'third',
            created_at: '2026-01-03T10:00:00Z',
          },
        ],
        next_cursor: 'older-than-3',
        has_more: true,
        total: 2,
      })
      .mockResolvedValueOnce({
        comments: [
          {
            id: 2,
            author_id: 2,
            content: 'second',
            created_at: '2026-01-02T10:00:00Z',
          },
        ],
        has_more: false,
      });
    render(MilestoneComments, { milestoneId: 50 });

    await fireEvent.click(await screen.findByTestId('milestone-comments-load-more'));

    await waitFor(() => {
      const rows = screen.getAllByTestId('milestone-comment');
      expect(rows.map((row) => row.dataset.commentId)).toEqual(['3', '2']);
    });
    expect(mocks.getComments).toHaveBeenLastCalledWith(50, {
      limit: 25,
      cursor: 'older-than-3',
    });
    expect(screen.queryByTestId('milestone-comments-load-more')).not.toBeInTheDocument();
    expect(screen.getByText('(2)')).toBeInTheDocument();
  });

  it('shows edit controls on foreign comments only to comment.edit_others holders on local milestones', async () => {
    const editableIds = async () => {
      const rows = await screen.findAllByTestId('milestone-comment');
      return rows
        .filter((row) => row.querySelector('[data-testid="milestone-comment-edit"]'))
        .map((row) => row.dataset.commentId);
    };

    mocks.canEditOthersComments.mockReturnValue(false);
    render(MilestoneComments, { milestoneId: 50, workspaceId: 4 });
    expect(await editableIds()).toEqual(['3', '1']);
    cleanup();

    mocks.canEditOthersComments.mockReturnValue(true);
    render(MilestoneComments, { milestoneId: 50, workspaceId: 4 });
    expect(await editableIds()).toEqual(['3', '2', '1']);
    expect(mocks.canEditOthersComments).toHaveBeenCalledWith(4);
    cleanup();

    // Global milestones have no workspace to grant the permission in.
    render(MilestoneComments, { milestoneId: 50 });
    expect(await editableIds()).toEqual(['3', '1']);
  });
});
