/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../../api.js';
import ItemDetailLinks from './ItemDetailLinks.svelte';

vi.mock('../../api.js', () => ({
  api: { requirements: { listKeys: vi.fn() } },
}));
vi.mock('../../stores/i18n.svelte.js', async () => {
  const { default: ru } = await import('../../locales/ru/requirements.js');
  return {
    t: (key) => key.startsWith('requirements.type.')
      ? ru.requirements.type[key.slice('requirements.type.'.length)] || key
      : key,
  };
});
vi.mock('../../router.js', () => ({
  currentRoute: {
    subscribe: (fn) => {
      fn({ view: 'item-detail' });
      return () => {};
    },
  },
  isMobileRoute: () => false,
}));

const requirementLink = {
  id: 1,
  link_type_id: 12,
  link_type_forward_label: 'implements',
  link_type_reverse_label: 'implemented by',
  source_type: 'item', source_id: 42,
  target_type: 'page', target_id: 91,
  target_workspace_id: 7, target_title: 'Login requirement',
};
const props = {
  itemId: 42,
  workspaceId: 7,
  linkTypes: [
    {
      id: 12, builtin_key: 'implements', name: 'Implements',
      display_name: 'Реализация',
      display_forward_label: 'реализует',
      display_reverse_label: 'реализуется',
    },
    { id: 14, builtin_key: 'page', name: 'Page' },
  ],
  itemLinks: [requirementLink],
};

describe('ItemDetailLinks requirement identity', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.requirements.listKeys).mockResolvedValue([
      {
        page_id: 91, requirement_number: 3, key: 'TE-DOC-3',
        requirement_type: 'functional_requirement',
      },
    ]);
  });
  afterEach(cleanup);

  it('shows requirements separately with their key, registry destination and add action', async () => {
    const onshowlinkmodal = vi.fn();
    render(ItemDetailLinks, { ...props, onshowlinkmodal });

    const section = await screen.findByTestId('linked-requirements-section');
    expect(within(section).getByRole('heading', { name: 'requirements.navTitle' })).toBeVisible();
    expect(screen.queryByRole('heading', { name: 'items.linkedPages' })).not.toBeInTheDocument();
    expect(within(section).getByText('TE-DOC-3')).toBeVisible();
    expect(within(section).getByTestId('linked-requirement-type')).toHaveTextContent('Функциональное требование');
    expect(within(section).getByTestId('linked-page-link-type')).toHaveTextContent('реализует');
    expect(within(section).getByTestId('linked-page-link-type')).toHaveAttribute('title', 'Реализация');
    expect(within(section).getByRole('link', { name: 'Login requirement' })).toHaveAttribute(
      'href', '/workspaces/7/requirements/3'
    );
    await fireEvent.click(within(section).getByRole('button', { name: 'common.add' }));
    expect(onshowlinkmodal).toHaveBeenCalledWith({ preselectLinkTypeId: 12 });
  });

  it('keeps ordinary pages in Pages even when they use the same implements relation', async () => {
    render(ItemDetailLinks, {
      ...props,
      itemLinks: [requirementLink, {
        ...requirementLink, id: 2, target_id: 92, target_title: 'Meeting notes',
      }],
    });

    const requirements = await screen.findByTestId('linked-requirements-section');
    const pages = screen.getByTestId('linked-pages-section');
    expect(within(requirements).getByRole('link', { name: 'Login requirement' })).toBeVisible();
    expect(within(requirements).queryByText('Meeting notes')).not.toBeInTheDocument();
    expect(within(pages).getByRole('heading', { name: 'items.linkedPages' })).toBeVisible();
    expect(within(pages).getByRole('link', { name: 'Meeting notes' })).toHaveAttribute(
      'href', '/workspaces/7/pages/92'
    );
    expect(within(pages).queryByText('Login requirement')).not.toBeInTheDocument();
    expect(within(pages).queryByTestId('linked-requirement-type')).not.toBeInTheDocument();
  });

  it('navigates from a modal to the requirement detail', async () => {
    const onnavigate = vi.fn();
    render(ItemDetailLinks, { ...props, isModal: true, onnavigate });

    const section = await screen.findByTestId('linked-requirements-section');
    await fireEvent.click(within(section).getByRole('link', { name: 'Login requirement' }));
    expect(onnavigate).toHaveBeenCalledWith('/workspaces/7/requirements/3');
  });

  it('uses the incoming relation when a source page and target item have the same id', async () => {
    vi.mocked(api.requirements.listKeys).mockResolvedValue([
      {
        page_id: 42, requirement_number: 3, key: 'TE-DOC-3',
        requirement_type: 'system_specification',
      },
    ]);
    render(ItemDetailLinks, {
      ...props,
      itemLinks: [{
        ...requirementLink,
        source_type: 'page', source_id: 42, source_workspace_id: 7,
        source_title: 'Incoming requirement',
        target_type: 'item', target_id: 42, target_title: 'Work item',
      }],
    });

    const section = await screen.findByTestId('linked-requirements-section');
    expect(within(section).getByRole('link', { name: 'Incoming requirement' })).toHaveAttribute(
      'href', '/workspaces/7/requirements/3'
    );
    expect(within(section).getByTestId('linked-page-link-type')).toHaveTextContent('реализуется');
    expect(within(section).getByTestId('linked-page-link-type')).toHaveAttribute('title', 'Реализация');
    expect(within(section).getByTestId('linked-requirement-type')).toHaveTextContent('Системная спецификация');
  });

  it('preserves custom relationship labels without display translations', async () => {
    render(ItemDetailLinks, {
      ...props,
      linkTypes: [{ id: 12, name: 'Custom relation', forward_label: 'согласует' }],
    });

    const section = await screen.findByTestId('linked-requirements-section');
    expect(within(section).getByTestId('linked-page-link-type')).toHaveTextContent('согласует');
    expect(within(section).getByTestId('linked-page-link-type')).toHaveAttribute('title', 'Custom relation');
  });
});
