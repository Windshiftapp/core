<script>
  import { ExternalLink, Loader } from '@lucide/svelte';
  import { navigate } from '../router.js';
  import { t } from '../stores/i18n.svelte.js';
  import Lozenge from '../components/Lozenge.svelte';
  import MobileHeader from './MobileHeader.svelte';
  import { loadMobileAssetSummary } from './mobileAssetData.js';

  let { assetId } = $props();

  let asset = $state(null);
  let loading = $state(true);
  let errored = $state(false);
  let loadToken = 0;

  function back() {
    if (window.history.length > 1) window.history.back();
    else navigate('/m/requirements');
  }

  function openInAssets() {
    navigate(`/assets/${assetId}`);
  }

  async function load(token) {
    loading = true;
    errored = false;
    try {
      asset = await loadMobileAssetSummary(assetId);
      if (token !== loadToken) return;
    } catch (err) {
      console.error('Failed to load asset:', err);
      if (token === loadToken) errored = true;
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  $effect(() => {
    const id = assetId;
    if (id == null) return;
    const token = ++loadToken;
    asset = null;
    load(token);
  });
</script>

<MobileHeader title={asset?.name ?? asset?.title ?? t('requirements.mobile.assetTitle')} onback={back} />

{#if loading}
  <div class="center" data-testid="mobile-asset-loading"><Loader class="spin" size={22} /></div>
{:else if errored || !asset}
  <div class="msg" data-testid="mobile-asset-error">
    <p>{t('requirements.mobile.assetLoadError')}</p>
    <button class="retry" onclick={() => load(++loadToken)} disabled={loading} type="button">
      {t('common.retry')}
    </button>
  </div>
{:else}
  <div class="detail" data-testid="mobile-asset-detail">
    <h2 class="title">{asset.name ?? asset.title}</h2>
    {#if asset.status_name}
      <Lozenge color={asset.status_color || 'neutral'}>{asset.status_name}</Lozenge>
    {/if}
    {#if asset.asset_type_name}
      <p class="meta">{t('requirements.mobile.assetType', { type: asset.asset_type_name })}</p>
    {/if}
    <button class="desktop-link" onclick={openInAssets} data-testid="mobile-asset-open-full" type="button">
      <ExternalLink size={16} />
      {t('requirements.mobile.openInAssets')}
    </button>
  </div>
{/if}

<style>
  .center { display: flex; justify-content: center; padding: 3rem; color: var(--ds-text-subtle); }
  :global(.spin) { animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .msg { padding: 3rem 1.25rem; text-align: center; color: var(--ds-text-subtle); }
  .msg p { margin: 0; }
  .retry {
    min-height: 40px;
    margin-top: 0.75rem;
    padding: 0.45rem 1rem;
    border: 1px solid var(--ds-interactive);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }

  .detail { padding: 0.875rem 0.875rem 2rem; }
  .title { margin: 0 0 0.75rem; font-size: 1.125rem; font-weight: 600; }
  .meta { margin: 0.5rem 0 0; font-size: 0.875rem; color: var(--ds-text-subtle); }

  .desktop-link {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    margin-top: 1.25rem;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text-link, var(--ds-interactive));
    font-size: 0.875rem;
    cursor: pointer;
  }
</style>
