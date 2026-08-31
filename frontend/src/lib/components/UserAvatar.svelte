<script>
  import { authStore, workspacesStore, attachmentStatus } from '../stores';
  import { navigate } from '../router.js';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { User, Home, Shield, Sun, Moon, Monitor, Download } from '@lucide/svelte';
  import { themeStore } from '../stores/theme.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api';
  import { installAvailable, requestInstall } from '../mobile/installClient.js';

  let {
    expanded = false,
    label = '',
    // minimal drops the items that navigate to the desktop app (My Workspace,
    // Profile, Security) — used on the mobile surface where those routes render
    // the full desktop UI. Theme + Sign Out remain.
    minimal = false,
    isOpen = $bindable(false),
    onOpenChange = null,
  } = $props();

  // Local state
  let loadingPersonalWorkspace = $state(false);

  // Subscribe to personal workspace from store
  const personalWorkspace = $derived($workspacesStore.personalWorkspace);

  // Only show avatar if attachments are enabled and user has an avatar
  const showAvatar = $derived(attachmentStatus.enabled && authStore.currentUser?.avatar_url);

  async function handleLogout() {
    await authStore.logout();
    // The App.svelte reactive statement will handle showing the login dialog
  }

  function handleProfileClick() {
    navigate('/profile');
  }

  // Load personal workspace on-demand
  async function loadPersonalWorkspaceIfNeeded() {
    if (!personalWorkspace && !loadingPersonalWorkspace && authStore.currentUser) {
      loadingPersonalWorkspace = true;
      try {
        await workspacesStore.loadPersonalWorkspace();
        // personalWorkspace will be updated automatically by the reactive statement
      } catch (error) {
        console.error('Failed to load personal workspace:', error);
      } finally {
        loadingPersonalWorkspace = false;
      }
    }
  }

  // Navigate to personal workspace (loads it first if needed)
  async function navigateToPersonalWorkspace() {
    if (!personalWorkspace) {
      await loadPersonalWorkspaceIfNeeded();
    }
    if (personalWorkspace) {
      navigate('/personal');
    } else {
      console.error('Could not load personal workspace');
    }
  }

  // Mobile-only: leave the mobile surface and remember the preference so the
  // post-login redirect doesn't bounce the user back to /m.
  function switchToDesktop() {
    try { localStorage.setItem('windshift-prefer-desktop', 'true'); } catch {}
    navigate('/');
  }

  async function selectThemeMode(mode) {
    if (themeStore.colorMode === mode) return;
    themeStore.setColorMode(mode);
    try {
      await api.userPreferences.update({ color_mode: mode });
    } catch (error) {
      console.warn('Failed to sync theme preference:', error);
    }
  }

  // Reactive theme icon based on current mode
  const themeIcon = $derived.by(() => {
    switch (themeStore.colorMode) {
      case 'light': return Sun;
      case 'dark': return Moon;
      default: return Monitor;
    }
  });

  // Reactive theme label based on current mode
  const themeLabel = $derived.by(() => {
    switch (themeStore.colorMode) {
      case 'light': return t('components.userAvatar.themeLight');
      case 'dark': return t('components.userAvatar.themeDark');
      default: return t('components.userAvatar.themeSystem');
    }
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div data-testid="user-avatar-menu" onmouseenter={loadPersonalWorkspaceIfNeeded} onfocusin={loadPersonalWorkspaceIfNeeded}>
<DropdownMenu
  triggerAvatar={showAvatar ? authStore.currentUser?.avatar_url : null}
  triggerText={expanded && label ? label : ''}
  triggerLabel={label || t('nav.profile')}
  triggerIcon={!showAvatar ? User : null}
  triggerIconClass="w-5 h-5"
  triggerClass={showAvatar
    ? "w-full pl-1.5 pr-3 h-10 rounded flex items-center cursor-pointer nav-button overflow-hidden"
    : "w-full px-3 h-10 rounded flex items-center cursor-pointer nav-button"
  }
  triggerGap={showAvatar || expanded ? "gap-3" : ""}
  triggerAlignment={showAvatar || expanded ? "start" : "center"}
  showChevron={false}
  triggerTestid="user-avatar-trigger"
  {isOpen}
  {onOpenChange}
  items={[
    ...((!minimal && authStore.currentUser) ? [{
      id: 'my-workspace',
      type: 'regular',
      icon: Home,
      iconColor: '#3b82f6',
      title: t('components.userAvatar.myWorkspace'),
      subtitle: t('components.userAvatar.myWorkspaceSubtitle'),
      onClick: navigateToPersonalWorkspace
    }, { type: 'divider' }] : []),
    ...(!minimal ? [{
      id: 'profile',
      type: 'regular',
      icon: User,
      iconColor: '#3b82f6',
      title: t('users.profile'),
      subtitle: t('components.userAvatar.profileSubtitle'),
      onClick: handleProfileClick
    },
    { type: 'divider' },
    {
      id: 'security',
      type: 'regular',
      icon: Shield,
      iconColor: '#3b82f6',
      title: t('components.userAvatar.security'),
      subtitle: t('components.userAvatar.securitySubtitle'),
      onClick: () => navigate('/security')
    },
    { type: 'divider' }] : []),
    {
      id: 'theme',
      testid: 'theme-menu',
      type: 'accordion',
      icon: themeIcon,
      iconColor: '#8b5cf6',
      title: t('components.userAvatar.themeTitle', { mode: themeLabel }),
      subItems: [
        {
          id: 'theme-light',
          testid: 'theme-light',
          icon: Sun,
          title: t('components.userAvatar.themeLight'),
          selected: themeStore.colorMode === 'light',
          onClick: () => selectThemeMode('light'),
          closeOnSelect: false
        },
        {
          id: 'theme-dark',
          testid: 'theme-dark',
          icon: Moon,
          title: t('components.userAvatar.themeDark'),
          selected: themeStore.colorMode === 'dark',
          onClick: () => selectThemeMode('dark'),
          closeOnSelect: false
        },
        {
          id: 'theme-system',
          testid: 'theme-system',
          icon: Monitor,
          title: t('components.userAvatar.themeSystem'),
          selected: themeStore.colorMode === 'system',
          onClick: () => selectThemeMode('system'),
          closeOnSelect: false
        }
      ]
    },
    { type: 'divider' },
    ...((minimal && installAvailable()) ? [{
      id: 'install-app',
      type: 'regular',
      icon: Download,
      iconColor: '#3b82f6',
      title: t('components.userAvatar.addToHomeScreen'),
      onClick: () => { requestInstall(); }
    }, { type: 'divider' }] : []),
    ...(minimal ? [{
      id: 'desktop-site',
      type: 'regular',
      icon: Monitor,
      iconColor: '#3b82f6',
      title: t('components.userAvatar.desktopSite'),
      onClick: switchToDesktop
    }, { type: 'divider' }] : []),
    {
      id: 'logout',
      type: 'regular',
      icon: User,
      iconColor: '#3b82f6',
      title: t('auth.signOut'),
      hoverClass: 'hover-danger',
      onClick: handleLogout
    }
  ]}
  maxWidth="max-w-xs"
  placement="right-start"
/>
</div>
