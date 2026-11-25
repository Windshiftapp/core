<script>
  import { onMount } from 'svelte';
  import { currentRoute, updateQueryParams } from '../router.js';
  import Tabs from '../components/Tabs.svelte';
  import SSOSettings from './SSOSettings.svelte';
  import IframePluginLoader from '../services/IframePluginLoader.svelte';
  import { loadExtensions, getExtensionsForPoint } from '../stores/extensions.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import KeyRound from '@lucide/svelte/icons/key-round';

  let { extensions: parentExtensions = null } = $props();

  let localExtensions = $state(null);

  const extensionsData = $derived(parentExtensions || localExtensions || {});

  const ssoTabExtensions = $derived(getExtensionsForPoint(extensionsData, 'admin.sso.tab'));

  // Active subtab lives in the URL query so deep links keep working, matching
  // the local-state-plus-URL-sync pattern used by ActionCapabilitiesManager.
  const validTabs = $derived(new Set(['oidc', ...ssoTabExtensions.map(ext => ext.id)]));

  const tabs = $derived([
    { id: 'oidc', label: t('settings.sso.oidcTab', 'OIDC'), icon: KeyRound },
    ...ssoTabExtensions.map(ext => ({
      id: ext.id,
      label: ext.label,
    }))
  ]);

  let subtab = $state('oidc');

  const activePlugin = $derived(ssoTabExtensions.find(ext => ext.id === subtab));

  function tabFromRoute(route) {
    const requestedTab = route.query?.subtab;
    return validTabs.has(requestedTab) ? requestedTab : 'oidc';
  }

  function setRouteTab(tab) {
    if (!validTabs.has(tab)) return;
    updateQueryParams({ subtab: tab === 'oidc' ? null : tab });
  }

  $effect(() => {
    subtab = tabFromRoute($currentRoute);
  });

  onMount(async () => {
    if (!parentExtensions) {
      /** @type {any} */
      const result = await loadExtensions();
      localExtensions = result || {};
    }
  });
</script>

<div class="space-y-4">
  <Tabs {tabs} bind:activeTab={subtab} onTabChange={({ tab }) => setRouteTab(tab)}>
    {#if activePlugin}
      {@const pluginName = activePlugin.pluginName || 'unknown'}
      {@const iframeSrc = `/api/plugins/${pluginName}/assets/${activePlugin.component || 'index.html'}`}
      <IframePluginLoader
        pluginName={activePlugin.label}
        src={iframeSrc}
      />
    {:else}
      <SSOSettings />
    {/if}
  </Tabs>
</div>
