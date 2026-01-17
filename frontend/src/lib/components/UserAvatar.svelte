<script>
  import { authStore, workspacesStore } from '../stores';
  import { navigate } from '../router.js';
  import { onMount } from 'svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { api } from '../api.js';
  import { User, Home, Shield, Camera, Sun, Moon, Monitor } from 'lucide-svelte';
  import { themeStore } from '../stores/theme.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  // Local state
  let loadingPersonalWorkspace = $state(false);
  let attachmentSettings = $state(null);
  let loadingAttachments = $state(false);

  // Subscribe to personal workspace from store
  const personalWorkspace = $derived($workspacesStore.personalWorkspace);

  // Generate user initials
  const userInitials = $derived(
    authStore.currentUser
      ? (authStore.currentUser.first_name?.[0]?.toUpperCase() || '') + (authStore.currentUser.last_name?.[0]?.toUpperCase() || '')
      : ''
  );

  // Check if attachments are enabled
  const attachmentsEnabled = $derived(attachmentSettings?.enabled && attachmentSettings?.attachment_path);

  onMount(async () => {
    try {
      loadingAttachments = true;
      attachmentSettings = await api.attachmentSettings.get();
    } catch (error) {
      console.warn('Failed to load attachment settings:', error);
    } finally {
      loadingAttachments = false;
    }
  });

  async function handleLogout() {
    await authStore.logout();
    // The App.svelte reactive statement will handle showing the login dialog
  }

  function handleAvatarClick() {
    // Navigate to profile page with avatar focus, or open avatar upload modal
    navigate('/profile?focus=avatar');
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
  triggerAvatar={authStore.currentUser?.avatar_url}
  triggerText={authStore.currentUser?.avatar_url ? '' : userInitials}
  triggerIcon={authStore.currentUser ? null : User}
  triggerClass={authStore.currentUser?.avatar_url
    ? "w-8 h-8 rounded-full cursor-pointer hover:opacity-80 transition-opacity overflow-hidden"
    : "w-8 h-8 rounded-full flex items-center justify-center cursor-pointer nav-button text-xs font-bold select-none"
  }
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
    ...(attachmentsEnabled ? [{
      id: 'avatar',
      type: 'regular',
      icon: User,
      iconColor: '#3b82f6',
      title: t('users.profile'),
      subtitle: t('components.userAvatar.profileSubtitle'),
      onClick: handleAvatarClick
    }, { type: 'divider' }] : []),
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
