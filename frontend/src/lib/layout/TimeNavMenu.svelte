<script>
  import IconClock from '@tabler/icons-svelte-runes/icons/clock';
  import IconPlayerPlay from '@tabler/icons-svelte-runes/icons/player-play';
  import IconPlayerStop from '@tabler/icons-svelte-runes/icons/player-stop';
  import Button from '../components/Button.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import { getShortcutDisplay, toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { currentRoute, navigate } from '../router.js';
  import { timerStore } from '../stores/timerStore.svelte.js';
  import { resolveTimeLogContext, resolveTimerWorkspaceId, timeLogStore } from '../stores/timeLogStore.svelte.js';
  import { stopTimerAndReview } from '../features/time/stopTimerAndReview.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, infoToast, warningToast } from '../stores/toasts.svelte.js';
  import { uiStore } from '../stores/ui.svelte.js';

  let {
    activeSurface = null,
    onSurfaceChange = () => {},
  } = $props();

  let projectQuery = $state('');

  const isActive = $derived($currentRoute.view === 'time');

  const projectOptions = $derived(
    timeLogStore.projects
      .filter((project) => project.status === 'Active')
      .filter((project) => {
        const query = projectQuery.trim().toLowerCase();
        return !query || project.name?.toLowerCase().includes(query);
      })
      .slice(0, 8)
  );

  async function startTimer(project = null) {
    const workspaceId = resolveTimerWorkspaceId();
    if (!workspaceId) {
      errorToast(t('time.entry.needWorkspace'));
      return;
    }
    if (!timerStore.canStart) {
      if (timerStore.activeTimer) warningToast(t('dialogs.alerts.timerAlreadyRunning'));
      else infoToast(t('dialogs.alerts.timerSyncing'));
      return;
    }
    const context = resolveTimeLogContext();
    try {
      await timerStore.start({
        workspace_id: workspaceId,
        project_id: project?.id ?? null,
        item_id: context.itemId ?? null,
        description: project?.name ?? '',
      });
    } catch (err) {
      if (!err.message?.includes('already running')) {
        errorToast(t('items.failedToStartTimer'), err.message);
      }
    }
  }

  async function stopTimer() {
    try {
      await stopTimerAndReview(timerStore, timeLogStore);
    } catch (err) {
      errorToast(t('dialogs.alerts.stopTimerFailed', { error: err.message }));
    }
  }

  const items = $derived.by(() => {
    const out = [];
    if (timerStore.activeTimer) {
      out.push({
        id: 'stop-timer',
        type: 'regular',
        icon: IconPlayerStop,
        title: t('time.stopTimer'),
        subtitle: timerStore.durationFormatted,
        onClick: stopTimer,
        testid: 'time-menu-stop-timer',
      });
    } else {
      out.push({
        id: 'start-timer',
        type: 'regular',
        icon: IconPlayerPlay,
        title: t('items.startTimer'),
        subtitle: t('time.menu.startTimerHint'),
        shortcut: getShortcutDisplay('global', 'startTimer'),
        onClick: () => startTimer(),
        testid: 'time-menu-start-timer',
      });
    }

    out.push({
      type: 'search',
      id: 'project-search',
      testid: 'time-menu-project-search',
      placeholder: t('placeholders.searchProjects'),
      value: projectQuery,
      onInput: (value) => { projectQuery = value; },
    });

    if (projectOptions.length > 0) {
      out.push({
        type: 'group',
        items: projectOptions.map((project) => ({
          id: `project-${project.id}`,
          title: project.name,
          onClick: () => startTimer(project),
          testid: `time-menu-project-${project.id}`,
        })),
      });
    }

    out.push({ type: 'divider' });

    if (timeLogStore.draftCount > 0) {
      out.push({
        id: 'drafts',
        type: 'regular',
        title: t('time.drafts'),
        badge: String(timeLogStore.draftCount),
        onClick: () => navigate('/time'),
        testid: 'time-menu-drafts',
      });
      out.push({ type: 'divider' });
    }

    out.push({
      type: 'group',
      items: [
        { id: 'time-entry', icon: IconClock, title: t('nav.timeAndProjects'), href: '/time' },
      ],
    });
    return out;
  });
</script>

<DropdownMenu
  triggerIcon={IconClock}
  triggerIconClass="w-5 h-5"
  triggerGap="gap-3"
  triggerText={$uiStore.navExpanded ? t('nav.timeAndProjects') : ''}
  triggerLabel={t('nav.timeAndProjects')}
  triggerClass="w-full h-10 rounded flex items-center {$uiStore.navExpanded ? 'justify-start px-3' : 'justify-center'} cursor-pointer nav-button {isActive ? 'nav-button-selected' : ''}"
  triggerTestid="time-nav-trigger"
  items={items}
  maxWidth="max-w-xs"
  showChevron={false}
  placement="right-start"
  iconOnly={!$uiStore.navExpanded}
  triggerAlignment={$uiStore.navExpanded ? 'start' : 'center'}
  isOpen={activeSurface === 'time'}
  onOpen={() => timeLogStore.ensurePickerData()}
  onOpenChange={(open) => onSurfaceChange(open ? 'time' : null)}
/>

<!-- Global start-timer shortcut. @github/hotkey ignores keystrokes that
     originate in form fields, so 'L' types normally while editing. -->
<Button
  class="sr-only"
  onclick={() => startTimer()}
  hotkeyConfig={{ key: toHotkeyString('global', 'startTimer'), guard: () => timerStore.canStart }}
>{t('items.startTimer')}</Button>
