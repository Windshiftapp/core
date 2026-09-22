/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { BUCKET_LABEL_KEYS } from '../commands/buckets.js';
import { mobileNavigationProvider } from '../commands/providers/mobileNavigationProvider.js';
import { rankCommands } from '../commands/rank.js';
import { i18n, SUPPORTED_LOCALES, t, translateError } from '../stores/i18n.svelte.js';
import MobileCommandPalette from './MobileCommandPalette.svelte';
import MobileConfirmSheet from './MobileConfirmSheet.svelte';
import MobileEditorPage from './MobileEditorPage.svelte';
import MobileListState from './MobileListState.svelte';
import MobileNav from './MobileNav.svelte';
import MobileOptionSheet from './MobileOptionSheet.svelte';
import MyWorkView from './MyWorkView.svelte';
import { mobilePalette } from './mobilePalette.svelte.js';

// Keep the real reactive i18n store and real locale indexes. Only account,
// network and navigation dependencies are isolated from this UI test.
const mockAiStore = vi.hoisted(() => ({ chatAvailable: true }));

vi.mock('../stores', async () => {
  const { writable } = await import('svelte/store');
  return {
    authStore: writable({ currentUser: { id: 1 } }),
    aiStore: mockAiStore,
    workspacesStore: writable({ personalWorkspace: null, regularWorkspaces: [] }),
  };
});
vi.mock('../router.js', async () => {
  const { writable } = await import('svelte/store');
  return {
    currentRoute: writable({ view: 'mobile-my-work' }),
    navigate: vi.fn(),
  };
});
vi.mock('../stores/notifications.js', async () => {
  const { writable } = await import('svelte/store');
  return { notifications: writable([]) };
});
vi.mock('../stores/timerStore.svelte.js', () => ({
  timerStore: { hasActive: false, activeTimer: null },
}));
vi.mock('../api.js', () => ({
  api: {
    items: { getAll: vi.fn().mockResolvedValue({ data: [] }) },
    homepage: { get: vi.fn().mockResolvedValue({ watched_items: [] }) },
    search: { items: vi.fn().mockResolvedValue([]) },
  },
}));
vi.mock('../commands/executor.js', () => ({ executeCommand: vi.fn().mockResolvedValue() }));
vi.mock('../components/UserAvatar.svelte', () => ({ default: () => {} }));

async function switchLocale(locale) {
  await i18n.setLocale(locale);
  await tick();
}

beforeEach(async () => {
  mockAiStore.chatAvailable = true;
  mobilePalette.close();
  await switchLocale('ko');
});

afterEach(() => {
  cleanup();
  mobilePalette.close();
  vi.clearAllMocks();
});

describe('mobile localization with real catalogs', () => {
  it('updates mounted navigation labels without changing destinations', async () => {
    render(MobileNav);
    const home = screen.getByTestId('mobile-nav-my-work');
    expect(home).toHaveTextContent('내 작업');
    expect(home).toHaveAttribute('href', '/m');
    expect(screen.getByTestId('mobile-nav-notifications')).toHaveTextContent('알림');
    await switchLocale('en');
    expect(home).toHaveTextContent('My Work');
    expect(screen.getByTestId('mobile-nav-notifications')).toHaveTextContent('Alerts');
    await switchLocale('ko');
    expect(home).toHaveTextContent('내 작업');
    expect(home).toHaveAttribute('aria-current', 'page');
  });

  it('updates mounted segments and their empty states', async () => {
    render(MyWorkView);
    await screen.findByText('현재 나에게 할당된 작업이 없습니다.');
    await fireEvent.click(screen.getByRole('tab', { name: '관심 항목' }));
    await screen.findByText('관심 등록한 항목이 없습니다.');
    await switchLocale('en');
    expect(screen.getByRole('tab', { name: 'Watched' })).toBeInTheDocument();
    expect(screen.getByText("You aren't watching any items.")).toBeInTheDocument();
  });

  it('updates list defaults and retry while retaining caller-supplied messages', async () => {
    const { rerender } = render(MobileListState);
    expect(screen.getByText('아직 항목이 없습니다.')).toBeInTheDocument();
    await switchLocale('en');
    expect(screen.getByText('Nothing here yet.')).toBeInTheDocument();
    const retry = vi.fn();
    await rerender({ errored: true, onretry: retry });
    await switchLocale('ko');
    expect(screen.getByText('불러오지 못했습니다.')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: t('common.retry') }));
    expect(retry).toHaveBeenCalledOnce();
    await rerender({ errored: true, errorMessage: 'External server detail' });
    await switchLocale('en');
    expect(screen.getByText('External server detail')).toBeInTheDocument();
  });

  it('updates confirmation defaults in an open sheet', async () => {
    render(MobileConfirmSheet, { isOpen: true, pushHistory: false });
    expect(screen.getByTestId('mobile-confirm-cancel')).toHaveTextContent('취소');
    await switchLocale('en');
    expect(screen.getByTestId('mobile-confirm-cancel')).toHaveTextContent('Cancel');
    expect(screen.getByTestId('mobile-confirm-accept')).toHaveTextContent('Confirm');
    expect(screen.getByRole('dialog')).toHaveAccessibleName(t('common.areYouSure'));
  });

  it('preserves custom confirmation labels and callback', async () => {
    const onconfirm = vi.fn();
    render(MobileConfirmSheet, {
      isOpen: true,
      pushHistory: false,
      title: 'Custom title',
      confirmLabel: 'Custom accept',
      cancelLabel: 'Custom cancel',
      onconfirm,
    });
    await switchLocale('en');
    await switchLocale('ko');
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Custom title');
    expect(screen.getByTestId('mobile-confirm-cancel')).toHaveTextContent('Custom cancel');
    await fireEvent.click(screen.getByRole('button', { name: 'Custom accept' }));
    expect(onconfirm).toHaveBeenCalledOnce();
  });

  it('updates editor defaults, busy state and custom labels', async () => {
    const { rerender } = render(MobileEditorPage);
    expect(screen.getByTestId('editor-save')).toHaveTextContent('저장');
    await switchLocale('en');
    expect(screen.getByTestId('editor-save')).toHaveTextContent('Save');
    await rerender({ saving: true });
    await switchLocale('ko');
    expect(screen.getByTestId('editor-save')).toBeDisabled();
    expect(screen.getByLabelText('저장 중…')).toBeInTheDocument();
    await rerender({ saving: false, saveLabel: 'Custom save', cancelLabel: 'Custom cancel' });
    await switchLocale('en');
    expect(screen.getByTestId('editor-save')).toHaveTextContent('Custom save');
    expect(screen.getByTestId('editor-cancel')).toHaveTextContent('Custom cancel');
  });

  it('updates picker defaults and search messages, preserving option names', async () => {
    render(MobileOptionSheet, {
      isOpen: true,
      options: [{ id: 7, name: 'External option' }],
      allowClear: true,
      multiple: true,
      searchable: true,
    });
    expect(screen.getByRole('option', { name: 'External option' })).toBeInTheDocument();
    expect(screen.getByTestId('mobile-sheet-clear')).toHaveTextContent(t('common.none'));
    await switchLocale('en');
    expect(screen.getByTestId('mobile-sheet-clear')).toHaveTextContent('None');
    expect(screen.getByTestId('mobile-sheet-done')).toHaveTextContent('Done');
    await fireEvent.input(screen.getByTestId('mobile-sheet-search'), {
      target: { value: 'no match' },
    });
    expect(screen.getByTestId('mobile-sheet-empty')).toHaveTextContent('No matches.');
    await switchLocale('ko');
    expect(screen.getByTestId('mobile-sheet-empty')).toHaveTextContent('일치하는 항목이 없습니다.');
  });

  it('updates the mounted command palette and searches Korean navigation labels', async () => {
    mobilePalette.open();
    render(MobileCommandPalette);
    expect(screen.getByRole('dialog')).toHaveAccessibleName('명령 팔레트');
    expect(screen.getByText('화면 이동')).toBeInTheDocument();
    expect(screen.getByText('내 작업')).toBeInTheDocument();
    await switchLocale('en');
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Command palette');
    expect(screen.getByText('My Work')).toBeInTheDocument();
    await switchLocale('ko');
    await fireEvent.input(screen.getByPlaceholderText('명령, 항목, 페이지 검색…'), {
      target: { value: '알림' },
    });
    await waitFor(() =>
      expect(screen.getByTestId('mobile-command-palette-option-m-notifications')).toHaveTextContent(
        '알림'
      )
    );
    expect(screen.queryByText('내 작업')).not.toBeInTheDocument();
  });

  it('keeps routes, English search aliases and the AI availability gate', () => {
    const commands = mobileNavigationProvider({ t });
    expect(commands.map(({ id, url }) => [id, url])).toEqual([
      ['m-my-work', '/m'],
      ['m-personal', '/m/personal'],
      ['m-pages', '/m/pages'],
      ['m-timer', '/m/timer'],
      ['m-notifications', '/m/notifications'],
      ['m-search', '/m/search'],
      ['m-chat', '/m/chat'],
    ]);
    expect(rankCommands('알림', commands).some((cmd) => cmd.id === 'm-notifications')).toBe(true);
    expect(rankCommands('pages', commands).some((cmd) => cmd.id === 'm-pages')).toBe(true);
    mockAiStore.chatAvailable = false;
    expect(mobileNavigationProvider({ t }).some((cmd) => cmd.id === 'm-chat')).toBe(false);
  });

  it('loads mobile namespaces through runtime indexes and interpolates user values', async () => {
    expect(t('mobile.search.empty', { query: 'ABC-123' })).toBe(
      '“ABC-123”에 해당하는 항목이나 페이지가 없습니다.'
    );
    expect(t('mobile.create.requiredField', { field: 'Customer field' })).toBe(
      'Customer field 항목은 필수입니다.'
    );
    expect(t('mobile.create.optionalFields', { count: 3 })).toBe('선택 필드 (3)');
    expect(translateError({ message: 'External server detail' })).toBe('External server detail');
    // Every shipped locale must resolve the full mobile surface — no English
    // fallback for missing mobile keys.
    for (const { code } of SUPPORTED_LOCALES) {
      await switchLocale(code);
      expect(t('mobile.create.item')).not.toBe('mobile.create.item');
      for (const key of Object.values(BUCKET_LABEL_KEYS)) expect(t(key)).not.toBe(key);
    }
  });
});
