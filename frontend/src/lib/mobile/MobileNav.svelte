<script>
  import { ListChecks, SquareCheckBig, BookOpen, Timer, Bell } from '@lucide/svelte';
  import { currentRoute } from '../router.js';
  import { notifications } from '../stores/notifications.js';
  import { timerStore } from '../stores/timerStore.svelte.js';

  const tabs = [
    { view: 'mobile-my-work', href: '/m', label: 'My Work', icon: ListChecks, testid: 'mobile-nav-my-work' },
    { view: 'mobile-personal', href: '/m/personal', label: 'Personal', icon: SquareCheckBig, testid: 'mobile-nav-personal' },
    { view: 'mobile-pages', href: '/m/pages', label: 'Pages', icon: BookOpen, testid: 'mobile-nav-pages' },
    { view: 'mobile-timer', href: '/m/timer', label: 'Timer', icon: Timer, testid: 'mobile-nav-timer' },
    { view: 'mobile-notifications', href: '/m/notifications', label: 'Alerts', icon: Bell, testid: 'mobile-nav-notifications' },
  ];

  const activeView = $derived($currentRoute.view);
  const unreadCount = $derived($notifications.filter((n) => !n.read).length);
</script>

<nav class="mobile-nav" data-testid="mobile-nav">
  {#each tabs as tab (tab.view)}
    {@const active = activeView === tab.view}
    <a
      href={tab.href}
      class="tab"
      class:active
      data-testid={tab.testid}
      aria-current={active ? 'page' : undefined}
    >
      <span class="icon-wrap">
        <tab.icon size={22} />
        {#if tab.view === 'mobile-notifications' && unreadCount > 0}
          <span class="badge" data-testid="mobile-nav-unread-badge">{unreadCount > 99 ? '99+' : unreadCount}</span>
        {/if}
        {#if tab.view === 'mobile-timer' && timerStore.hasActive}
          <span class="dot" data-testid="mobile-nav-timer-dot"></span>
        {/if}
      </span>
      <span class="label">{tab.label}</span>
    </a>
  {/each}
</nav>

<style>
  .mobile-nav {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: var(--z-sticky, 200);
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    background-color: var(--ds-surface-raised);
    border-top: 1px solid var(--ds-border);
    padding-bottom: env(safe-area-inset-bottom, 0px);
  }

  .tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
    min-height: 44px;
    padding: 0.5rem 0;
    text-decoration: none;
    color: var(--ds-text-subtle);
    transition: color var(--duration-fast, 100ms) ease;
  }

  .tab.active {
    color: var(--ds-interactive);
  }

  .icon-wrap {
    position: relative;
    display: inline-flex;
  }

  .label {
    font-size: 0.6875rem;
    line-height: 1;
  }

  .badge {
    position: absolute;
    top: -6px;
    right: -10px;
    min-width: 16px;
    height: 16px;
    padding: 0 4px;
    border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-danger, #e5484d);
    color: #fff;
    font-size: 0.625rem;
    font-weight: var(--font-semibold, 600);
    line-height: 16px;
    text-align: center;
  }

  .dot {
    position: absolute;
    top: -3px;
    right: -5px;
    width: 8px;
    height: 8px;
    border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-success, #4cb782);
  }
</style>
