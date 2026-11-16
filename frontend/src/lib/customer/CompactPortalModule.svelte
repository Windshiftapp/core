<script>
  import { onMount, onDestroy } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import { portalCustomizationStore as portalStore, portalCatalogStore } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { portalRequestsStore, portalApprovalsStore, portalDraftsStore } from '../stores/portalActivity.svelte.js';
  import LazyRootView from '../components/LazyRootView.svelte';
  import LazyRootDialog from '../components/LazyRootDialog.svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalLoginModal from '../portal/PortalLoginModal.svelte';
  import GlobalConfirmDialog from '../dialogs/GlobalConfirmDialog.svelte';

  let { slug, kind, formId = null, articleId = null, draftsEnabled = true, onidentitychange = null, ondefaultunavailable = null } = $props();
  let ready = $state(false);
  let error = $state('');
  let selectedForm = $state(null);
  let selectedReport = $state(null);
  let prefill = $state({});
  const loaders = {
    requests: () => import('../portal/PortalMyRequests.svelte'),
    drafts: () => import('../portal/PortalMyDrafts.svelte'),
    approvals: () => import('../portal/PortalMyApprovals.svelte'),
    profile: () => import('../portal/PortalProfile.svelte'),
    'knowledge-base': () => import('./CompactKnowledge.svelte'),
    article: () => import('../portal/PortalKBArticle.svelte'),
    'asset-reports': () => import('../portal/PortalSections.svelte'),
  };
  const loadForm = () => import('../dialogs/RequestFormModal.svelte');
  const loadReport = () => import('../dialogs/AssetReportFormModal.svelte');
  let disposed = false;

  async function load() {
    try {
      ready = false;
      const [, bootstrap] = await Promise.all([portalStore.loadPortal(slug), portalAuthStore.checkAuth(slug)]);
      if (disposed) return;
      portalStore.hydrateUserBootstrap(bootstrap);
      if (!bootstrap?.authenticated) { portalStore.showLoginDialog = true; ready = true; return; }
      if (kind === 'requests') {
        portalRequestsStore.setVisible(true);
        if ($currentRoute.query?.id) await portalRequestsStore.loadAndView($currentRoute.query.id);
        else await portalRequestsStore.load();
      }
      if (kind === 'approvals') { portalApprovalsStore.setVisible(true); if ($currentRoute.query?.id) await portalApprovalsStore.loadAndView($currentRoute.query.id); else await portalApprovalsStore.load(); }
      if (kind === 'drafts') { portalDraftsStore.setVisible(true); await portalDraftsStore.load(); }
      if (kind === 'forms') {
        await portalCatalogStore.loadRequestTypes();
        if (disposed) return;
        const id = formId;
        const forms = portalCatalogStore.requestTypes;
        const target = id ? forms.find(form => form.id === id) : forms.length === 1 && !$currentRoute.query?.choose ? forms[0] : null;
        if (id && !target) { if (ondefaultunavailable) { ondefaultunavailable(); return; } error = 'This request form is unavailable.'; }
        if (target) selectedForm = target;
        prefill = Object.fromEntries(Object.entries($currentRoute.query || {}).filter(([key]) => key.startsWith('prefill.') && !key.endsWith('.label')).map(([key, value]) => [key.slice(8), value]));
      }
      if (kind === 'asset-reports') await portalCatalogStore.loadAssetReports();
      if (!disposed) ready = true;
    } catch (err) { if (!disposed) { error = err.message; ready = true; } }
  }

  onMount(load);
  onDestroy(() => { disposed = true; portalStore.reset(); portalAuthStore.reset(); });
</script>

<div class="mx-auto max-w-6xl p-4 sm:p-6" data-testid={`compact-module-${kind}`}>
  {#if error}<AlertBox variant="error" message={error} />{/if}
  {#if !ready}<Spinner />{:else if $portalAuthStore.isAuthenticated}
    {#if kind === 'forms'}
      <h2 class="mb-4 text-lg font-semibold">New request</h2>
      <div class="flex flex-col items-start gap-2">{#each portalCatalogStore.requestTypes as form (form.id)}<Button dataTestid={`compact-request-type-${form.id}`} onclick={() => navigate(`/portal/${slug}/request/${form.id}`)}>{form.name}</Button>{/each}</div>
    {:else if loaders[kind]}
      <LazyRootView loader={kind === 'knowledge-base' && articleId ? loaders.article : loaders[kind]} label="Portal module" componentProps={kind === 'knowledge-base' && articleId ? { pageId: articleId, onunavailable: ondefaultunavailable } : kind === 'drafts' ? { onresume: ({ requestType }) => navigate(`/portal/${slug}/request/${requestType.id}`) } : kind === 'asset-reports' ? { onOpenAssetReportForm: report => selectedReport = report, onOpenRequestForm: form => navigate(`/portal/${slug}/request/${form.id}`) } : {}} />
    {/if}
  {/if}
  <PortalLoginModal onloginsuccess={async () => { await onidentitychange?.(); if (!disposed) await load(); }} />
  <GlobalConfirmDialog />
  {#if selectedForm}
    <LazyRootDialog loader={loadForm} isOpen={true} componentProps={{ requestType: selectedForm, portalSlug: slug, prefill, draftsEnabled, onsubmitted: id => navigate(`/portal/${slug}?view=requests&id=${id}`), onclose: () => { selectedForm = null; navigate(`/portal/${slug}?module=forms&choose=1`); } }} />
  {/if}
  {#if selectedReport}<LazyRootDialog loader={loadReport} isOpen={true} componentProps={{ report: selectedReport, portalSlug: slug, onclose: () => selectedReport = null }} />{/if}
</div>
