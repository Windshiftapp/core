<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { correlationKey } = $props();
  let failed = $state(false);
  let resolving = $state(false);

  async function resolveLink() {
    if (resolving) return;
    resolving = true;
    failed = false;
    try {
      let decodedCorrelationKey = correlationKey;
      try {
        decodedCorrelationKey = decodeURIComponent(correlationKey);
      } catch {
        // Keep the original value so the API returns the same unavailable
        // result as it does for any other malformed or unknown key.
      }
      const destination = await api.zammadTickets.resolve(decodedCorrelationKey);
      const workspaceId = Number(destination?.workspace_id);
      const itemId = Number(destination?.item_id);
      if (!Number.isInteger(workspaceId) || workspaceId <= 0 || !Number.isInteger(itemId) || itemId <= 0) {
        throw new Error('Invalid Zammad link destination');
      }
      navigate(`/workspaces/${workspaceId}/items/${itemId}`, { replace: true });
    } catch (error) {
      console.error('Failed to resolve Zammad ticket link:', error);
      failed = true;
    } finally {
      resolving = false;
    }
  }

  onMount(() => {
    void resolveLink();
  });
</script>

<div class="flex min-h-[50vh] items-center justify-center p-6">
  <div class="max-w-md text-center">
    {#if failed}
      <p class="mb-4" style="color: var(--ds-text-danger);">{t('zammad.returnLinkFailed')}</p>
      <Button variant="primary" onclick={resolveLink}>{t('common.retry')}</Button>
    {:else}
      <Spinner class="mx-auto mb-4" />
      <p style="color: var(--ds-text-subtle);">{t('zammad.returnLinkResolving')}</p>
    {/if}
  </div>
</div>
