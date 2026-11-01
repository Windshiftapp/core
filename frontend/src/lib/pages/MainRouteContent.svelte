<script>
  import Button from '../components/Button.svelte';
  import DelayedMount from '../components/DelayedMount.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Footer from '../layout/Footer.svelte';
  import PermissionGuard from '../layout/PermissionGuard.svelte';
  import { authStore } from '../stores/auth.svelte.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import { uiStore } from '../stores/ui.svelte.js';
  import UnauthorizedAccess from './UnauthorizedAccess.svelte';
  import {
    getMainAppLazyState,
    getMainAppRouteProps,
    isMainAppFooterView,
    resolveMainAppRoute,
  } from './mainAppRoutes.js';

  let { view, route, lazyComponents } = $props();

  const routeEntry = $derived(resolveMainAppRoute(view));
  const wrapper = $derived(routeEntry.config?.wrapper || null);
  const routeState = $derived(getMainAppLazyState(lazyComponents, view));
  // Only render the footer on opted-in routes and only once the route has
  // rendered successfully, so it never appears under a spinner or error state.
  const showFooter = $derived(
    isMainAppFooterView(view) &&
      Boolean(routeState.component) &&
      !routeState.loading &&
      !routeState.error &&
      !$uiStore.reviewFullscreen
  );
  // Footer routes need a flex column so `mt-auto` can pin the footer on short
  // pages. Put that column inside the scroll container rather than on it: a
  // fixed-height flex container shrinks tall content, whose overflow then
  // paints over the footer.
  const footerLayout = $derived(
    isMainAppFooterView(view) && !$uiStore.reviewFullscreen
  );
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
  <DelayedMount>
    <div class="flex items-center justify-center h-full" data-testid="root-view-loading">
      <div class="text-center">
        <Spinner class="mx-auto mb-4" />
        <p class="text-ds-text-subtle">{message}</p>
      </div>
    </div>
  </DelayedMount>
{/snippet}

{#snippet errorState(message, retryFn)}
  <div class="flex items-center justify-center h-full">
    <div class="text-center">
      <p class="text-ds-text-danger">{message}</p>
      <Button variant="primary" onclick={retryFn} class="mt-4">
        {t('nav.retry')}
      </Button>
    </div>
  </div>
{/snippet}

{#snippet routeFooter()}
  {#if showFooter}
    <Footer />
  {/if}
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
{:else if view === 'delegated-admin'}
  <!-- The shell owns its own full-height layout and per-surface guards. -->
  {@render lazyLoadedComponent(view, routeProps)}
{:else if view === 'workspace-actions'}
  <div class="h-full" style="background-color: var(--ds-surface); height: calc(100vh - 56px);">
    {@render lazyLoadedComponent(view, routeProps)}
  </div>
{:else if routeEntry.config}
  {#if wrapper === 'surface-full'}
    <div class="h-full min-h-0 overflow-y-auto" style="background-color: var(--ds-surface);">
      {#if footerLayout}
        <div class="flex min-h-full flex-col">
          {@render lazyLoadedComponent(view, routeProps)}
          {@render routeFooter()}
        </div>
      {:else}
        {@render lazyLoadedComponent(view, routeProps)}
      {/if}
    </div>
  {:else if wrapper === 'surface-padded'}
    {#if footerLayout}
      <div class="flex min-h-full flex-col">
        <div class="p-6" style="background-color: var(--ds-surface);">
          {@render lazyLoadedComponent(view, routeProps)}
        </div>
        {@render routeFooter()}
      </div>
    {:else}
      <div class="p-6" style="background-color: var(--ds-surface);">
        {@render lazyLoadedComponent(view, routeProps)}
      </div>
    {/if}
  {:else if wrapper === 'surface-admin'}
    <div class="h-full min-h-0 overflow-y-auto px-16 py-12" style="background-color: var(--ds-surface);">
      {#if footerLayout}
        <div class="flex min-h-full flex-col">
          {@render lazyLoadedComponent(view, routeProps)}
          {@render routeFooter()}
        </div>
      {:else}
        {@render lazyLoadedComponent(view, routeProps)}
      {/if}
    </div>
  {:else if wrapper === 'surface'}
    {#if footerLayout}
      <div class="flex min-h-full flex-col">
        <div style="background-color: var(--ds-surface);">
          {@render lazyLoadedComponent(view, routeProps)}
        </div>
        {@render routeFooter()}
      </div>
    {:else}
      <div style="background-color: var(--ds-surface);">
        {@render lazyLoadedComponent(view, routeProps)}
      </div>
    {/if}
  {:else}
    {#if footerLayout}
      <div class="flex min-h-full flex-col">
        {@render lazyLoadedComponent(view, routeProps)}
        {@render routeFooter()}
      </div>
    {:else}
      {@render lazyLoadedComponent(view, routeProps)}
    {/if}
  {/if}
{:else}
  {@render lazyLoadedComponent('workspaces', { showAdminHeader: false })}
{/if}
