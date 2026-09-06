/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import ZammadConnectionManager from './ZammadConnectionManager.svelte';

const mocks = vi.hoisted(() => ({
  getConnections: vi.fn(),
  getWorkspaces: vi.fn(),
  getStatuses: vi.fn(),
  testConnection: vi.fn(),
  updateConnection: vi.fn(),
}));

vi.mock('../api.js', () => ({
  api: {
    zammadConnections: {
      getAll: mocks.getConnections,
      test: mocks.testConnection,
      update: mocks.updateConnection,
    },
    workspaces: { getAll: mocks.getWorkspaces },
    statuses: { getAll: mocks.getStatuses },
  },
}));

vi.mock('../stores/i18n.svelte.js', () => ({ t: (key) => key }));
vi.mock('../stores/toasts.svelte.js', () => ({
  successToast: vi.fn(),
  errorToast: vi.fn(),
  warningToast: vi.fn(),
}));
vi.mock('../composables/useConfirm.js', () => ({ confirm: vi.fn() }));

const connection = {
  id: 'zammad-dev',
  slug: 'zammad-dev',
  name: 'Zammad Dev',
  enabled: true,
  base_url: 'https://zammad.example.test',
  auth_method: 'oauth',
  oauth_client_id: 'client-id',
  has_oauth_client_secret: true,
  oauth_connected: true,
  default_group_id: 0,
  default_group_name: 'Windshift',
  allowed_groups: [
    { id: 2, name: 'Support' },
    { id: 3, name: 'Escalations' },
  ],
  default_customer: 'windshift@example.test',
  correlation_field: 'windshift_item_key',
  closed_state_ids: [4],
  applies_to_all_workspaces: true,
  workspace_ids: [],
};

describe('ZammadConnectionManager', () => {
  beforeAll(() => {
    Object.defineProperty(Element.prototype, 'animate', {
      configurable: true,
      value: vi.fn(() => {
        const animation = {
          cancel: vi.fn(),
          currentTime: 0,
          effect: {},
          onfinish: null,
          playState: 'finished',
        };
        queueMicrotask(() => animation.onfinish?.());
        return animation;
      }),
    });
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getConnections.mockResolvedValue([connection]);
    mocks.getWorkspaces.mockResolvedValue([]);
    mocks.getStatuses.mockResolvedValue([]);
    mocks.testConnection.mockResolvedValue({
      metadata: {
        groups: [
          { id: 2, name: 'Windshift' },
          { id: 3, name: 'Windshift Escalation' },
        ],
        states: [],
        correlation_field_verified: true,
      },
    });
    mocks.updateConnection.mockResolvedValue(connection);
  });

  afterEach(cleanup);

  it('persists the selected default group ID and name', async () => {
    const { container } = render(ZammadConnectionManager);
    await screen.findByText('Zammad Dev');

    await fireEvent.click(screen.getByRole('button', { name: 'zammad.testConnection' }));
    await waitFor(() => expect(mocks.testConnection).toHaveBeenCalledWith('zammad-dev'));

    const editButton = container.querySelector('svg.lucide-pen')?.closest('button');
    expect(editButton).toBeTruthy();
    await fireEvent.click(editButton);

    const dialog = screen.getByRole('dialog');
    await fireEvent.change(within(dialog).getAllByRole('combobox')[0], {
      target: { value: '2' },
    });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'common.update' }));

    await waitFor(() =>
      expect(mocks.updateConnection).toHaveBeenCalledWith(
        'zammad-dev',
        expect.objectContaining({ default_group_id: 2, default_group_name: 'Windshift' })
      )
    );
  });

  it('labels and lets administrators repair migrated groups without names', async () => {
    mocks.getConnections.mockResolvedValue([
      {
        ...connection,
        default_group_id: 99,
        default_group_name: '',
        allowed_groups: [
          { id: 2, name: 'Windshift' },
          { id: 99, name: '' },
        ],
      },
    ]);
    mocks.testConnection.mockResolvedValue({
      metadata: {
        groups: [
          { id: 2, name: 'Windshift' },
          { id: 99, name: '' },
        ],
        states: [],
        correlation_field_verified: false,
        group_catalog_verified: false,
      },
    });

    const { container } = render(ZammadConnectionManager);
    await screen.findByText('Zammad Dev');
    await fireEvent.click(screen.getByRole('button', { name: 'zammad.testConnection' }));
    await waitFor(() => expect(mocks.testConnection).toHaveBeenCalledWith('zammad-dev'));

    const editButton = container.querySelector('svg.lucide-pen')?.closest('button');
    expect(editButton).toBeTruthy();
    await fireEvent.click(editButton);

    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getAllByText('zammad.unverifiedGroup')).toHaveLength(2);
    const defaultGroupSelect = /** @type {HTMLSelectElement} */ (
      within(dialog).getAllByRole('combobox')[0]
    );
    const defaultGroupOptions = Array.from(defaultGroupSelect.options).map(
      (option) => option.textContent
    );
    expect(defaultGroupOptions).toEqual(['Windshift', 'zammad.unverifiedGroup']);
    expect(defaultGroupSelect.value).toBe('99');

    await fireEvent.input(within(dialog).getByLabelText('settings.groups.groupName'), {
      target: { value: 'Legacy Support' },
    });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'common.update' }));

    await waitFor(() =>
      expect(mocks.updateConnection).toHaveBeenCalledWith(
        'zammad-dev',
        expect.objectContaining({
          default_group_id: 99,
          default_group_name: 'Legacy Support',
          allowed_groups: expect.arrayContaining([{ id: 99, name: 'Legacy Support' }]),
        })
      )
    );
  });
});
