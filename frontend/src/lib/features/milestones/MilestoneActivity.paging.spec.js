/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import MilestoneActivity from './MilestoneActivity.svelte';

const mocks = vi.hoisted(() => ({ getActivity: vi.fn() }));

vi.mock('../../api/milestones.js', () => ({
  milestones: { getActivity: mocks.getActivity },
}));
vi.mock('../../stores/i18n.svelte.js', () => ({ t: (key) => key }));

// A request the test settles by hand, to control the order responses arrive in.
function deferred() {
  /** @type {(value: any) => void} */
  let resolve = () => {};
  const promise = new Promise((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

// One feed page; each entry's actor name identifies it in the rendered list.
function feed(names, hasMore = false) {
  return {
    entries: names.map((name) => ({
      id: `milestone_comment:${name}`,
      type: 'milestone_comment_added',
      actor_kind: 'user',
      actor_name: name,
      occurred_at: '2026-01-01T10:00:00Z',
    })),
    hasMore,
  };
}

// Let a settled request run its handler and Svelte update the DOM.
async function settle() {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await tick();
}

async function shownActors(expected) {
  await waitFor(() => {
    const rows = screen.queryAllByTestId('milestone-activity-entry');
    expect(rows.map((row) => row.querySelector('.font-medium')?.textContent)).toEqual(expected);
  });
}

describe('MilestoneActivity', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => cleanup());

  it('drops a Load more page that arrives after the feed reloaded', async () => {
    const olderPage = deferred();
    mocks.getActivity
      .mockResolvedValueOnce(feed(['old-1'], true))
      .mockReturnValueOnce(olderPage.promise)
      .mockResolvedValueOnce(feed(['new-1'], true));

    const { rerender } = render(MilestoneActivity, { milestoneId: 7, reloadToken: 0 });
    await shownActors(['old-1']);

    await fireEvent.click(screen.getByTestId('milestone-activity-load-more'));
    await rerender({ milestoneId: 7, reloadToken: 1 });
    await shownActors(['new-1']);

    olderPage.resolve(feed(['old-2']));
    await settle();
    await shownActors(['new-1']);
    // The reloaded feed still asks for its own page 2.
    expect(screen.getByTestId('milestone-activity-load-more')).toBeInTheDocument();
  });

  it('keeps the latest reload when an earlier one answers last', async () => {
    const first = deferred();
    const second = deferred();
    mocks.getActivity.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { rerender } = render(MilestoneActivity, { milestoneId: 7, reloadToken: 0 });
    await rerender({ milestoneId: 7, reloadToken: 1 });

    second.resolve(feed(['latest']));
    await shownActors(['latest']);
    first.resolve(feed(['stale']));
    await settle();
    await shownActors(['latest']);
  });

  it('clears a Load more error once a retry succeeds', async () => {
    mocks.getActivity
      .mockResolvedValueOnce(feed(['first'], true))
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(feed(['second']));
    vi.spyOn(console, 'error').mockImplementation(() => {});

    render(MilestoneActivity, { milestoneId: 7 });
    await shownActors(['first']);

    await fireEvent.click(screen.getByTestId('milestone-activity-load-more'));
    expect(await screen.findByText('dialogs.alerts.failedToLoad')).toBeInTheDocument();

    await fireEvent.click(screen.getByTestId('milestone-activity-load-more'));
    await shownActors(['first', 'second']);
    expect(screen.queryByText('dialogs.alerts.failedToLoad')).not.toBeInTheDocument();
    expect(mocks.getActivity).toHaveBeenLastCalledWith(7, { page: 2, pageSize: 50 });
  });
});
