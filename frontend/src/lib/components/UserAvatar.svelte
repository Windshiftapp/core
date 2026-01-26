<script>
  import { authStore, workspacesStore, attachmentStatus } from '../stores';
  import { navigate } from '../router.js';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { User, Home, Shield, Camera, Sun, Moon, Monitor } from 'lucide-svelte';
  import { themeStore } from '../stores/theme.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    expanded = false,
    label = ''
  } = $props();

  // Local state
  let loadingPersonalWorkspace = $state(false);

  // Subscribe to personal workspace from store
  const personalWorkspace = $derived($workspacesStore.personalWorkspace);

  // Generate user initials
  const userInitials = $derived(
    authStore.currentUser
      ? (authStore.currentUser.first_name?.[0]?.toUpperCase() || '') + (authStore.currentUser.last_name?.[0]?.toUpperCase() || '')
      : ''
  );

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

  // Theme toggle function
  async function handleThemeToggle() {
    themeStore.cycleMode();
    // Sync to backend (fire and forget)
    try {
      await api.userPreferences.update({ color_mode: themeStore.colorMode });
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

<!-- svelte-ignore a11y-mouse-events-have-key-events -->
<div on:mouseenter={loadPersonalWorkspaceIfNeeded}>
<DropdownMenu
  triggerAvatar={showAvatar ? authStore.currentUser?.avatar_url : null}
  triggerText={expanded && label ? label : (showAvatar ? '' : userInitials)}
  triggerIcon={expanded && !showAvatar ? User : null}
  triggerIconClass="w-5 h-5"
  triggerClass={expanded
    ? "w-full px-3 h-10 rounded flex items-center cursor-pointer nav-button"
    : (showAvatar
      ? "w-8 h-8 rounded-full cursor-pointer hover:opacity-80 transition-opacity overflow-hidden"
      : "w-8 h-8 rounded-full flex items-center justify-center cursor-pointer nav-button text-xs font-bold select-none"
    )
  }
  triggerGap={expanded ? "gap-3" : ""}
  triggerAlignment={expanded ? "start" : "center"}
  showChevron={false}
  items={[
    ...(authStore.currentUser ? [{
      id: 'my-workspace',
      type: 'regular',
      icon: Home,
      iconColor: '#3b82f6',
      title: t('components.userAvatar.myWorkspace'),
      subtitle: t('components.userAvatar.myWorkspaceSubtitle'),
      onClick: navigateToPersonalWorkspace
    }, { type: 'divider' }] : []),
    {
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
    { type: 'divider' },
    {
      id: 'theme',
      type: 'regular',
      icon: themeIcon,
      iconColor: '#8b5cf6',
      title: t('components.userAvatar.themeTitle', { mode: themeLabel }),
      subtitle: t('components.userAvatar.themeCycle'),
      onClick: handleThemeToggle
    },
    { type: 'divider' },
    {
      id: 'logout',
      type: 'regular',
      icon: User,
      iconColor: '#3b82f6',
      title: t('auth.signOut'),
      hoverClass: 'hover:bg-red-50 hover:text-red-700',
      onClick: handleLogout
    }
  ]}
  maxWidth="max-w-xs"
  placement="right-start"
/>
</div>
