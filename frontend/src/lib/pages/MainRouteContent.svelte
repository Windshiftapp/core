<script>
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import PermissionGuard from '../layout/PermissionGuard.svelte';
  import { authStore, workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import UnauthorizedAccess from './UnauthorizedAccess.svelte';
  import {
    getMainAppLazyState,
    getMainAppRouteProps,
    resolveMainAppRoute,
  } from './mainAppRoutes.js';

  let { view, route, lazyComponents } = $props();

  const routeEntry = $derived(resolveMainAppRoute(view));
  const wrapper = $derived(routeEntry.config?.wrapper || null);
  const routeProps = $derived.by(() =>
    getMainAppRouteProps(view, route, {
      currentUser: authStore.currentUser,
      moduleSettings: $moduleSettings,
      personalWorkspaceId: $workspacesStore.personalWorkspace?.id,
    })
  );

  $effect(() => {
    void lazyComponents.load(routeEntry.key || 'workspaces');
  });
</script>

{#snippet loadingState(message)}
  <div class="flex items-center justify-center h-full">
    <div class="text-center">
      <Spinner class="mx-auto mb-4" />
      <p class="text-gray-600">{message}</p>
    </div>
  </div>
{/snippet}

{#snippet errorState(message, retryFn)}
  <div class="flex items-center justify-center h-full">
    <div class="text-center">
      <p class="text-red-600">{message}</p>
      <Button variant="primary" onclick={retryFn} class="mt-4">
        {t('nav.retry')}
      </Button>
    </div>
  </div>
{/snippet}

{#snippet lazyLoadedComponent(lazyView, props)}
  {@const state = getMainAppLazyState(lazyComponents, lazyView)}

  {#if state.loading}
    {@render loadingState(state.config?.loadingMsg || 'Loading...')}
  {:else if state.component}
    {@const LazyComponent = state.component}
    <LazyComponent {...props} />
  {:else if state.error}
    {@render errorState(
      state.config?.errorMsg || 'Failed to load component',
      () => lazyComponents.retry(state.loaderKey)
    )}
  {:else}
    {@render loadingState(state.config?.loadingMsg || 'Loading...')}
  {/if}
{/snippet}

{#if view === 'admin'}
  <!-- Keep one guard/component tree for every admin route. Switching between
       Channels and another admin tab must not remount Admin and reset its
       independently scrollable navigation. -->
  <PermissionGuard requireSystemAdmin={!route.path.startsWith('/admin/channels')}>
    {@render lazyLoadedComponent(view, routeProps)}
    {#snippet fallback(requiredPermissionDisplay)}
      <UnauthorizedAccess
        message="You need system administrator privileges to access the administration panel."
        requiredPermission={requiredPermissionDisplay}
      />
    {/snippet}
  </PermissionGuard>
{:else if view === 'workspace-actions'}
  <div class="h-full" style="background-color: var(--ds-surface); height: calc(100vh - 56px);">
    {@render lazyLoadedComponent(view, routeProps)}
  </div>
{:else if routeEntry.config}
  {#if wrapper === 'surface-full'}
    <div class="h-full min-h-0 overflow-y-auto" style="background-color: var(--ds-surface);">
      {@render lazyLoadedComponent(view, routeProps)}
    </div>
  {:else if wrapper === 'surface-padded'}
    <div class="p-6" style="background-color: var(--ds-surface);">
      {@render lazyLoadedComponent(view, routeProps)}
    </div>
  {:else if wrapper === 'surface-admin'}
    <div class="px-16 py-12 flex-1 overflow-y-auto" style="background-color: var(--ds-surface);">
      {@render lazyLoadedComponent(view, routeProps)}
    </div>
  {:else if wrapper === 'surface'}
    <div style="background-color: var(--ds-surface);">
      {@render lazyLoadedComponent(view, routeProps)}
    </div>
  {:else}
    {@render lazyLoadedComponent(view, routeProps)}
  {/if}
{:else}
  {@render lazyLoadedComponent('workspaces', { showAdminHeader: false })}
{/if}
