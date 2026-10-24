/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import MilestonePages from './MilestonePages.svelte';

const mocks = vi.hoisted(() => ({
  getPageLinks: vi.fn(),
  linkPage: vi.fn(),
  unlinkPage: vi.fn(),
  searchPages: vi.fn(),
  errorToast: vi.fn(),
}));

vi.mock('../../api/milestones.js', () => ({
  milestones: {
    getPageLinks: mocks.getPageLinks,
    linkPage: mocks.linkPage,
    unlinkPage: mocks.unlinkPage,
  },
}));
vi.mock('../../api/pages.js', () => ({
  pages: { searchPages: mocks.searchPages },
}));

vi.mock('../../stores/i18n.svelte.js', () => ({ t: (key) => key }));
vi.mock('../../stores/toasts.svelte.js', () => ({ errorToast: mocks.errorToast }));

const page = { id: 42, title: 'Spec page' };

async function openDialog(props = {}) {
  render(MilestonePages, { milestoneId: 1, workspaceId: 7, canEdit: true, ...props });
  const add = await screen.findByTestId('milestone-page-add');
  await fireEvent.click(add);
  const dialog = await screen.findByTestId('milestone-page-link-modal');
  const input = within(dialog).getByTestId('milestone-page-picker');
  // Modal focuses the picker shortly after opening.
  await waitFor(() => expect(input).toHaveFocus());
  return { add, dialog, input };
}

async function choosePage(input) {
  await fireEvent.click(input);
  await fireEvent.input(input, { target: { value: 'Spec' } });
  const dropdown = await screen.findByTestId('picker-dropdown');
  await fireEvent.click(await within(dropdown).findByText('Spec page', {}, { timeout: 2000 }));
}

function expectDialogClosed() {
  return waitFor(() => expect(screen.queryByTestId('milestone-page-link-modal')).toBeNull());
}

describe('MilestonePages link dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getPageLinks.mockResolvedValue([]);
    mocks.searchPages.mockResolvedValue({ results: [page] });
    mocks.linkPage.mockResolvedValue({
      id: 9,
      page_id: 42,
      page_title: 'Spec page',
      workspace_id: 7,
    });
  });

  afterEach(() => cleanup());

  it('opens a modal dialog from "+ Add" instead of an inline picker', async () => {
    const { add, dialog } = await openDialog();

    expect(add).toHaveAttribute('aria-haspopup', 'dialog');
    expect(dialog).toHaveAttribute('role', 'dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(within(dialog).getByRole('heading', { name: 'items.addPage' })).toBeInTheDocument();
    expect(within(dialog).getByTestId('milestone-page-link-cancel')).toHaveTextContent(
      'common.cancel'
    );
    const confirm = within(dialog).getByTestId('milestone-page-link-confirm');
    expect(confirm).toHaveTextContent('items.addPage');
    expect(confirm).toBeDisabled();
    // The picker lives in the dialog, not inline in the header card.
    expect(
      within(screen.getByTestId('milestone-pages')).queryByTestId('milestone-page-picker')
    ).toBeNull();
  });

  it('searches pages in the milestone workspace only', async () => {
    const { input } = await openDialog();
    await fireEvent.click(input);
    await fireEvent.input(input, { target: { value: 'Spec' } });

    await waitFor(() => expect(mocks.searchPages).toHaveBeenCalled());
    expect(mocks.searchPages.mock.calls.every(([workspaceId]) => workspaceId === 7)).toBe(true);
  });

  it('links the chosen page, closes and lists it', async () => {
    const { add, dialog, input } = await openDialog();
    await choosePage(input);

    const confirm = within(dialog).getByTestId('milestone-page-link-confirm');
    await waitFor(() => expect(confirm).toBeEnabled());
    await fireEvent.click(confirm);

    await waitFor(() => expect(mocks.linkPage).toHaveBeenCalledWith(1, 42));
    await expectDialogClosed();
    const rows = screen.getAllByTestId('milestone-page-row');
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent('Spec page');
    expect(add).toHaveFocus();
  });

  it('keeps the dialog open when linking fails', async () => {
    mocks.linkPage.mockRejectedValue(new Error('Page is already linked to this milestone'));
    const { dialog, input } = await openDialog();
    await choosePage(input);

    const confirm = within(dialog).getByTestId('milestone-page-link-confirm');
    await waitFor(() => expect(confirm).toBeEnabled());
    await fireEvent.click(confirm);

    await waitFor(() =>
      expect(mocks.errorToast).toHaveBeenCalledWith(
        'Page is already linked to this milestone',
        'errors.failedToUpdate'
      )
    );
    expect(screen.getByTestId('milestone-page-link-modal')).toBeInTheDocument();
    expect(screen.queryAllByTestId('milestone-page-row')).toHaveLength(0);
  });

  it('rejects a page that is already linked', async () => {
    mocks.getPageLinks.mockResolvedValue([
      { id: 9, page_id: 42, page_title: 'Spec page', workspace_id: 7 },
    ]);
    const { dialog, input } = await openDialog();
    await choosePage(input);

    expect(await within(dialog).findByTestId('milestone-page-already-linked')).toHaveTextContent(
      'pickers.alreadyLinked'
    );
    const confirm = within(dialog).getByTestId('milestone-page-link-confirm');
    expect(confirm).toBeDisabled();
    await fireEvent.click(confirm);
    expect(mocks.linkPage).not.toHaveBeenCalled();
  });

  it('closes on Cancel without linking and refocuses "+ Add"', async () => {
    const { add, dialog, input } = await openDialog();
    await choosePage(input);

    await fireEvent.click(within(dialog).getByTestId('milestone-page-link-cancel'));

    await expectDialogClosed();
    expect(mocks.linkPage).not.toHaveBeenCalled();
    expect(add).toHaveFocus();
  });

  it('closes on Escape from the search box and refocuses "+ Add"', async () => {
    const { add, input } = await openDialog();

    await fireEvent.keyDown(input, { key: 'Escape' });

    await expectDialogClosed();
    expect(mocks.linkPage).not.toHaveBeenCalled();
    expect(add).toHaveFocus();
  });

  it('closes only the dropdown on the first Escape, then the dialog', async () => {
    const { add, input } = await openDialog();
    await fireEvent.click(input);
    await waitFor(() => expect(screen.getByTestId('picker-dropdown')).toBeInTheDocument());

    await fireEvent.keyDown(input, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByTestId('picker-dropdown')).toBeNull());
    expect(screen.getByTestId('milestone-page-link-modal')).toBeInTheDocument();

    await fireEvent.keyDown(input, { key: 'Escape' });
    await expectDialogClosed();
    expect(add).toHaveFocus();
  });

  it('closes on a click outside and refocuses "+ Add"', async () => {
    const { add, dialog } = await openDialog();

    // The Modal backdrop is the dialog element itself.
    await fireEvent.click(dialog);

    await expectDialogClosed();
    expect(mocks.linkPage).not.toHaveBeenCalled();
    expect(add).toHaveFocus();
  });

  it('opens a fresh dialog after closing', async () => {
    const { dialog, input } = await openDialog();
    await choosePage(input);
    await fireEvent.click(within(dialog).getByTestId('milestone-page-link-cancel'));
    await expectDialogClosed();

    await fireEvent.click(screen.getByTestId('milestone-page-add'));
    const reopened = await screen.findByTestId('milestone-page-link-modal');
    expect(within(reopened).getByTestId('milestone-page-link-confirm')).toBeDisabled();
  });

  it('gives viewers no "+ Add" and no remove buttons', async () => {
    mocks.getPageLinks.mockResolvedValue([
      { id: 9, page_id: 42, page_title: 'Spec page', workspace_id: 7 },
    ]);
    render(MilestonePages, { milestoneId: 1, workspaceId: 7, canEdit: false });

    expect(await screen.findByTestId('milestone-page-row')).toBeInTheDocument();
    expect(screen.queryByTestId('milestone-page-add')).toBeNull();
    expect(screen.queryByTestId('milestone-page-unlink')).toBeNull();
    expect(screen.queryByTestId('milestone-page-link-modal')).toBeNull();
  });
});
