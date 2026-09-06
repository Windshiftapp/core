/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ZammadLinkResolver from './ZammadLinkResolver.svelte';

const mocks = vi.hoisted(() => ({ resolve: vi.fn(), navigate: vi.fn() }));

vi.mock('../../api.js', () => ({ api: { zammadTickets: { resolve: mocks.resolve } } }));
vi.mock('../../router.js', () => ({ navigate: mocks.navigate }));
vi.mock('../../stores/i18n.svelte.js', () => ({ t: (key) => key }));

describe('ZammadLinkResolver', () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(cleanup);

  it('replaces the resolver route with the current item destination', async () => {
    mocks.resolve.mockResolvedValue({ workspace_id: 7, item_id: 42 });

    render(ZammadLinkResolver, { correlationKey: 'windshift%3Aprovider%3ATST-42' });

    await waitFor(() =>
      expect(mocks.navigate).toHaveBeenCalledWith('/workspaces/7/items/42', { replace: true })
    );
    expect(mocks.resolve).toHaveBeenCalledWith('windshift:provider:TST-42');
  });

  it('keeps a retry action when the link cannot be resolved', async () => {
    mocks.resolve.mockRejectedValueOnce(new Error('not found'));
    mocks.resolve.mockResolvedValueOnce({ workspace_id: 7, item_id: 42 });

    render(ZammadLinkResolver, { correlationKey: 'windshift:provider:TST-42' });

    const retry = await screen.findByRole('button', { name: 'common.retry' });
    expect(screen.getByText('zammad.returnLinkFailed')).toBeInTheDocument();
    await fireEvent.click(retry);
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledOnce());
  });
});
