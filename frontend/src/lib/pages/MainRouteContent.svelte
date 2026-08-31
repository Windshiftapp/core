<script>
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Channels from '../features/channels/Channels.svelte';
  import Collections from '../features/collections/Collections.svelte';
  import CollectionsList from '../features/collections/CollectionsList.svelte';
  import PermissionGuard from '../layout/PermissionGuard.svelte';
  import { authStore, workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import TeamDetail from '../teams/TeamDetail.svelte';
  import TeamsList from '../teams/TeamsList.svelte';
  import Customers from '../workspaces/Customers.svelte';
  import WorkspaceSettings from '../workspaces/WorkspaceSettings.svelte';
  import ApprovalsInbox from './ApprovalsInbox.svelte';
  import About from './About.svelte';
  import ApiDocs from './ApiDocs.svelte';
  import CliAuthorize from './CliAuthorize.svelte';
  import Hub from '../layout/Hub.svelte';
  import NotFound from './NotFound.svelte';
  import NotificationsPage from './NotificationsPage.svelte';
  import OAuthAuthorize from './OAuthAuthorize.svelte';
  import SearchPage from './SearchPage.svelte';
  import Security from './Security.svelte';
  import UnauthorizedAccess from './UnauthorizedAccess.svelte';
  import UserProfile from './UserProfile.svelte';
  import Workspaces from '../workspaces/Workspaces.svelte';
  import {
    getMainAppLazyState,
    getMainAppRouteProps,
    resolveMainAppRoute,
    WORKSPACE_SETTINGS_TABS,
    WORKSPACE_SETTINGS_VIEWS,
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
    if (routeEntry.key) void lazyComponents.load(routeEntry.key);
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

{#if view === 'workspaces'}
  <Workspaces showAdminHeader={false} />
{:else if WORKSPACE_SETTINGS_VIEWS.has(view)}
  <div class="p-6" style="background-color: var(--ds-surface);">
    <WorkspaceSettings
      workspaceId={route.params.id}
      activeTab={WORKSPACE_SETTINGS_TABS[view] || 'general'}
    />
  </div>
{:else if view === 'collections-list'}
  <CollectionsList />
{:else if view === 'collections-edit'}
  <Collections collectionId={route.params.id} />
{:else if view === 'channels'}
  <div class="h-full min-h-0 overflow-y-auto" style="background-color: var(--ds-surface);">
    <Channels />
  </div>
{:else if view === 'hub' || view === 'hub-inbox'}
  <div style="background-color: var(--ds-surface);">
    <Hub />
  </div>
{:else if view === 'organizations' || view === 'organization-contact-detail'}
  <Customers />
{:else if view === 'teams-list'}
  <div class="p-6" style="background-color: var(--ds-surface);">
    <TeamsList />
  </div>
{:else if view === 'team-detail'}
  <div class="p-6" style="background-color: var(--ds-surface);">
    <TeamDetail teamId={route.params.id} section={route.params.section || 'overview'} />
  </div>
{:else if view === 'notifications'}
  <NotificationsPage />
{:else if view === 'approvals-inbox'}
  <ApprovalsInbox />
{:else if view === 'search'}
  <SearchPage />
{:else if view === 'profile'}
  <div class="p-6" style="background-color: var(--ds-surface);">
    <UserProfile />
  </div>
{:else if view === 'security'}
  <div class="p-6" style="background-color: var(--ds-surface);">
    <Security />
  </div>
{:else if view === 'about'}
  <About />
{:else if view === 'api-docs'}
  <ApiDocs />
{:else if view === 'cli-authorize'}
  <CliAuthorize />
{:else if view === 'oauth-authorize'}
  <OAuthAuthorize />
{:else if view === '404'}
  <div class="p-6" style="background-color: var(--ds-surface);">
    <NotFound />
  </div>
{:else if view === 'admin'}
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
  {:else}
    {@render lazyLoadedComponent(view, routeProps)}
  {/if}
{:else}
  <Workspaces showAdminHeader={false} />
{/if}
