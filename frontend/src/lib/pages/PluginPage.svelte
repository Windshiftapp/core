<script>
  import { onMount } from 'svelte';
  import IframePluginLoader from '../services/IframePluginLoader.svelte';
  import PluginModalContainer from '../layout/PluginModalContainer.svelte';
  import NotFound from './NotFound.svelte';
  import { extensions, loadExtensions } from '../stores/extensions.svelte.js';
  import { findPluginNavItem } from '../navigation/pluginNavigation.js';

  // A plugin's `main.nav` page, opened from the main sidebar (navigation/pluginNavigation.js).
  let { pluginName = '', extensionId = '' } = $props();

  let loaded = $state(false);
  const item = $derived(findPluginNavItem(extensions.value, pluginName, extensionId));

  onMount(async () => {
    await loadExtensions({ quiet: true });
    loaded = true;
  });
</script>

{#if item}
  {#key item.src}
    <IframePluginLoader pluginName={item.label} src={item.src} />
  {/key}
{:else if loaded}
  <!-- Unknown, disabled or removed plugin -->
  <NotFound />
{/if}

<PluginModalContainer />
