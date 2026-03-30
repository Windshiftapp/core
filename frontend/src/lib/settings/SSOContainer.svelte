<script>
  import { onMount } from 'svelte';
  import { currentRoute } from '../router.js';
  import TabNav from '../components/TabNav.svelte';
  import SSOSettings from './SSOSettings.svelte';
  import IframePluginLoader from '../services/IframePluginLoader.svelte';
  import { loadExtensions, getExtensionsForPoint } from '../stores/extensions.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let { extensions: parentExtensions = null } = $props();

  let localExtensions = $state(null);

  const extensionsData = $derived(parentExtensions || localExtensions || {});

  const ssoTabExtensions = $derived(getExtensionsForPoint(extensionsData, 'admin.sso.tab'));

  const tabs = $derived([
    { id: 'oidc', label: t('settings.sso.oidcTab', 'OIDC') },
    ...ssoTabExtensions.map(ext => ({
      id: ext.id,
      label: ext.label,
    }))
  ]);

  let subtab = $derived($currentRoute.query?.subtab || 'oidc');

  const activePlugin = $derived(ssoTabExtensions.find(ext => ext.id === subtab));

  onMount(async () => {
    if (!parentExtensions) {
      /** @type {any} */
      const result = await loadExtensions();
      localExtensions = result || {};
    }
  });
</script>

<div class="space-y-6">
  {#if tabs.length > 1}
    <TabNav {tabs} basePath="/admin/sso" defaultTab="oidc" />
  {/if}

  <div>
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
  </div>
</div>
