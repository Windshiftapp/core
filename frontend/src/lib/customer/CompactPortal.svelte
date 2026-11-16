<script>
  import { currentRoute, navigate } from '../router.js';
  import { portalBoards } from '../api/portalBoards.js';
  import { customerAccess } from '../api/customerAccess.js';
  import CustomerHeader from './CustomerHeader.svelte';
  import Button from '../components/Button.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import LazyRootView from '../components/LazyRootView.svelte';
  import CustomerLogin from './CustomerLogin.svelte';
  import { compactModules, compactTarget } from './compactNavigation.js';

  let { experience } = $props();
  let accessError = $state(null);
  let showLogin = $state(false);
  let loginStatus = $state(null);
  let rejectedDefault = $state(null);
  const loadBoard = () => import('../pages/PublicBoard.svelte');
  const loadModule = () => import('./CompactPortalModule.svelte');
  const slug = $derived(experience.slug);
  const modules = $derived(compactModules(experience));
  const target = $derived(compactTarget(rejectedDefault === experience.default_module ? { ...experience, default_module: '' } : experience, $currentRoute, modules));
  const active = $derived(target.kind);
  const placementId = $derived(active?.startsWith('board:') ? active.slice(6) : null);
  const client = $derived(placementId ? portalBoards.client(slug, placementId) : null);
  const navigation = $derived(modules.map(item => ({ ...item, active: item.id === active })));
  const moduleEnabled = $derived(placementId ? experience.modules.includes('boards') : experience.modules.includes(active));

  function selectItem(key) {
    navigate(`/portal/${slug}/boards/${placementId}${key ? `/items/${encodeURIComponent(key)}` : ''}`, { replace: false });
  }

  async function signIn() {
    try { loginStatus = await customerAccess.status('portal', slug); showLogin = true; }
    catch (err) { accessError = err.status || 403; }
  }

  async function identityChanged() {
    experience = await portalBoards.experience(slug);
    accessError = null;
    showLogin = false;
  }
  async function signOut() {
    try { await customerAccess.login('portal', slug, 'logout'); window.location.reload(); }
    catch (err) { accessError = err.status || 403; }
  }
  // Recheck identity/policy after another tab signs in/out; never keep stale customer modules.
  function refreshIdentity() { void identityChanged().catch(() => { experience = { ...experience, authenticated: false, identity_key: 0, boards: [] }; }); }
</script>

<svelte:window onfocus={event => { if (event.target === window) refreshIdentity(); }} />

<div class="flex min-h-screen flex-col bg-ds-surface text-ds-text" data-testid="compact-portal">
  <CustomerHeader title={experience.title} logo={experience.logo_url} {navigation}>
    {#snippet actions()}
      <!-- shortcut-guard-exempt: Customer navigation, not internal item creation; available by Tab and Enter without registering app-wide creation keys in a public shell. -->
      {#if experience.modules.includes('forms') && active === 'requests'}<Button dataTestid="compact-new-request" onclick={() => navigate(`/portal/${slug}?module=forms`)}>New request</Button>{/if}
      {#if experience.is_internal}<Button variant="ghost" href="/" dataTestid="compact-back-to-app">Back to app</Button>
      {:else if !experience.authenticated}<Button variant="ghost" dataTestid="compact-sign-in" onclick={signIn}>Sign in</Button>
      {:else}<Button variant="ghost" dataTestid="compact-sign-out" onclick={signOut}>Sign out</Button>{/if}
    {/snippet}
  </CustomerHeader>
  {#if accessError}<AlertBox variant="warning" message={accessError === 401 ? 'Sign in to view this board.' : 'Your account does not have access to this board.'} />{/if}
  {#if showLogin && loginStatus}<CustomerLogin kind="portal" {slug} status={loginStatus} onlogin={identityChanged} />{/if}
  <main class="min-w-0 flex-1" data-testid="compact-portal-content">
    {#if !active}<EmptyState title="No content available" description="There are no accessible modules in this portal." />
    {:else if !moduleEnabled}<EmptyState title="Content unavailable" description="This module is not enabled for this portal." />
    {:else if placementId}
      {#key `${slug}:${placementId}:${experience.identity_key}:${experience.revision}`}<LazyRootView loader={loadBoard} label="Board" componentProps={{ slug, client, embedded: true, initialItemKey: $currentRoute.params?.itemKey || null, onitemchange: selectItem, onaccessdenied: status => { accessError = status; if (status === 401) void signIn(); } }} />{/key}
    {:else}
      {#key `${slug}:${active}:${target.formId || ''}:${target.articleId || ''}:${experience.identity_key}:${experience.revision}:${$currentRoute.path}:${$currentRoute.query?.id || ''}`}<LazyRootView loader={loadModule} label="Portal module" componentProps={{ slug, kind: active, formId: target.formId, articleId: target.articleId, draftsEnabled: experience.modules.includes('drafts'), onidentitychange: identityChanged, ondefaultunavailable: !$currentRoute.params?.pageId && !$currentRoute.params?.requestTypeId ? () => { rejectedDefault = experience.default_module; } : null }} />{/key}
    {/if}
  </main>
</div>
